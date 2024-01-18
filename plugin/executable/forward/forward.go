/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package fastforward

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const PluginType = "forward"

var defaultQueryTimeout time.Duration

func init() {
	defaultQueryTimeout = getDefaultQueryTimeout()
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
	sequence.MustRegExecQuickSetup(PluginType, quickSetup)
}

const (
	maxConcurrentQueries = 3
)

type Args struct {
	Upstreams  []UpstreamConfig `yaml:"upstreams"`
	Concurrent int              `yaml:"concurrent"`
	QTime      int              `yaml:"qtime"`
	// Global options.
	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
}

type UpstreamConfig struct {
	Tag            string `yaml:"tag"`
	Addr           string `yaml:"addr"` // Required.
	DialAddr       string `yaml:"dial_addr"`
	IdleTimeout    int    `yaml:"idle_timeout"`
	MaxConns       int    `yaml:"max_conns"`
	EnablePipeline bool   `yaml:"enable_pipeline"`

	Socks5       string `yaml:"socks5"`
	SoMark       int    `yaml:"so_mark"`
	BindToDevice string `yaml:"bind_to_device"`
}

func getDefaultQueryTimeout() time.Duration {
	QUERY_TIME := os.Getenv("QUERY_TIME")
	if QUERY_TIME == "" {
		return time.Second * 3
	}
	duration, err := time.ParseDuration(QUERY_TIME)
	if err != nil {
		fmt.Printf("Failed to parse QUERY_TIME value: %v\n", err)
		return time.Second * 3
	}
	return duration
}
func Init(bp *coremain.BP, args any) (any, error) {
	f, err := NewForward(args.(*Args), Opts{Logger: bp.L(), MetricsTag: bp.Tag()})
	if err != nil {
		return nil, err
	}
	return f, nil
}

var _ sequence.Executable = (*Forward)(nil)
var _ sequence.QuickConfigurableExec = (*Forward)(nil)

type Forward struct {
	args *Args

	logger       *zap.Logger
	us           map[*upstreamWrapper]struct{}
	tag2Upstream map[string]*upstreamWrapper // for fast tag lookup only.
}

type Opts struct {
	Logger     *zap.Logger
	MetricsTag string
}

// NewForward inits a Forward from given args.
// args must contain at least one upstream.
func NewForward(args *Args, opt Opts) (*Forward, error) {
	if len(args.Upstreams) == 0 {
		return nil, errors.New("no upstream is configured")
	}
	if opt.Logger == nil {
		opt.Logger = zap.NewNop()
	}

	f := &Forward{
		args:         args,
		logger:       opt.Logger,
		us:           make(map[*upstreamWrapper]struct{}),
		tag2Upstream: make(map[string]*upstreamWrapper),
	}

	applyGlobal := func(c *UpstreamConfig) {
		utils.SetDefaultString(&c.Socks5, args.Socks5)
		utils.SetDefaultUnsignNum(&c.SoMark, args.SoMark)
		utils.SetDefaultString(&c.BindToDevice, args.BindToDevice)
	}

	for i, c := range args.Upstreams {
		if len(c.Addr) == 0 {
			return nil, fmt.Errorf("#%d upstream invalid args, addr is required", i)
		}
		applyGlobal(&c)

		uw := newWrapper(c, opt.MetricsTag)
		uOpt := upstream.Opt{
			DialAddr:       c.DialAddr,
			Socks5:         c.Socks5,
			SoMark:         c.SoMark,
			BindToDevice:   c.BindToDevice,
			IdleTimeout:    time.Duration(c.IdleTimeout) * time.Second,
			MaxConns:       c.MaxConns,
			EnablePipeline: c.EnablePipeline,
			Logger:         opt.Logger,
		}

		u, err := upstream.NewUpstream(c.Addr, uOpt)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to init upstream #%d: %w", i, err)
		}
		uw.u = u
		f.us[uw] = struct{}{}

		if len(c.Tag) > 0 {
			if _, dup := f.tag2Upstream[c.Tag]; dup {
				_ = f.Close()
				return nil, fmt.Errorf("duplicated upstream tag %s", c.Tag)
			}
			f.tag2Upstream[c.Tag] = uw
		}
	}

	return f, nil
}

func (f *Forward) Exec(ctx context.Context, qCtx *query_context.Context) (err error) {
	r, err := f.exchange(ctx, qCtx, f.us)
	if err != nil {
		return err
	}
	qCtx.SetResponse(r)
	return nil
}

// QuickConfigureExec format: [upstream_tag]...
func (f *Forward) QuickConfigureExec(args string) (any, error) {
	var us map[*upstreamWrapper]struct{}
	if len(args) == 0 { // No args, use all upstreams.
		us = f.us
	} else { // Pick up upstreams by tags.
		us = make(map[*upstreamWrapper]struct{})
		for _, tag := range strings.Fields(args) {
			u := f.tag2Upstream[tag]
			if u == nil {
				return nil, fmt.Errorf("cannot find upstream by tag %s", tag)
			}
			us[u] = struct{}{}
		}
	}
	var execFunc sequence.ExecutableFunc = func(ctx context.Context, qCtx *query_context.Context) error {
		r, err := f.exchange(ctx, qCtx, us)
		if err != nil {
			return nil
		}
		qCtx.SetResponse(r)
		return nil
	}
	return execFunc, nil
}

func (f *Forward) Close() error {
	for u := range f.us {
		_ = (*u).Close()
	}
	return nil
}

func (f *Forward) exchange(ctx context.Context, qCtx *query_context.Context, us map[*upstreamWrapper]struct{}) (*dns.Msg, error) {
	if len(us) == 0 {
		return nil, errors.New("no upstream to exchange")
	}
	queryTimeout := defaultQueryTimeout
	if f.args.QTime > 0 {
		queryTimeout = time.Duration(f.args.QTime) * time.Millisecond
	}
	mcq := f.args.Concurrent
	if mcq <= 0 {
		mcq = 1
	}
	if mcq > maxConcurrentQueries {
		mcq = maxConcurrentQueries
	}

	type res struct {
		r   *dns.Msg
		u   *upstreamWrapper
		err error
	}

	resChan := make(chan res)
	done := make(chan struct{})
	defer close(done)

	qc := qCtx.Q().Copy()
	uqid := qCtx.Id()
	sent := 0
	for u := range us {
		if sent > mcq {
			break
		}
		sent++

		u := u
		go func() {
			// Give each upstream a fixed timeout to finsh the query.
			upstreamCtx, cancel := context.WithTimeout(context.Background(), queryTimeout)
			defer cancel()
			r, err := u.ExchangeContext(upstreamCtx, qc)
			if err != nil {
				f.logger.Warn(
					"upstream error",
					zap.Uint32("uqid", uqid),
					zap.Inline((*queryInfo)(qc)),
					zap.String("upstream", u.name()),
					zap.Error(err),
				)
			}
			select {
			case resChan <- res{r: r, err: err, u: u}:
			case <-done:
			}
		}()
	}

	es := new(utils.Errors)
	for i := 0; i < sent; i++ {
		select {
		case res := <-resChan:
			r, u, err := res.r, res.u, res.err
			if err != nil {
				es.Append(&upstreamErr{
					upstreamName: u.name(),
					err:          err,
				})
				continue
			}
			if r.Rcode == dns.RcodeSuccess {
				return r, nil
			}
		case <-ctx.Done():
			es.Append(fmt.Errorf("exchange: %w", ctx.Err()))
			return nil, es
		}
	}
	return nil, es
}

func quickSetup(bq sequence.BQ, s string) (any, error) {
	args := new(Args)
	args.Concurrent = maxConcurrentQueries
	for _, u := range strings.Fields(s) {
		args.Upstreams = append(args.Upstreams, UpstreamConfig{Addr: u})
	}
	return NewForward(args, Opts{Logger: bq.L()})
}
