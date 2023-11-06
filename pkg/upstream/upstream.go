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

package upstream

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/transport"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

// Upstream represents a DNS upstream.
type Upstream interface {
	// ExchangeContext exchanges query message m to the upstream, and returns
	// response. It MUST NOT keep or modify m.
	ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error)

	io.Closer
}

type Opt struct {
	// DialAddr specifies the address the upstream will
	// actually dial to.
	DialAddr string

	// Socks5 specifies the socks5 proxy server that the upstream
	// will connect though.
	// Not implemented for udp upstreams and doh upstreams with http/3.
	Socks5 string

	// SoMark sets the socket SO_MARK option in unix system.
	SoMark int

	// BindToDevice sets the socket SO_BINDTODEVICE option in unix system.
	BindToDevice string

	// IdleTimeout specifies the idle timeout for long-connections.
	// Available for TCP, DoT, DoH.
	// If negative, TCP, DoT will not reuse connections.
	// Default: TCP, DoT: 10s , DoH: 30s.
	IdleTimeout time.Duration

	// EnablePipeline enables query pipelining support as RFC 7766 6.2.1.1 suggested.
	// Available for TCP, DoT upstream with IdleTimeout >= 0.
	EnablePipeline bool

	// MaxConns limits the total number of connections, including connections
	// in the dialing states.
	// Implemented for TCP/DoT pipeline enabled upstreams and DoH upstreams.
	// Default is 2.
	MaxConns int

	// Logger specifies the logger that the upstream will use.
	Logger *zap.Logger

	// EventObserver can observe connection events.
	// Note: Not Implemented for HTTP/3 upstreams.
	EventObserver EventObserver
}

func NewUpstream(addr string, opt Opt) (Upstream, error) {
	if opt.Logger == nil {
		opt.Logger = mlog.Nop()
	}
	if opt.EventObserver == nil {
		opt.EventObserver = nopEO{}
	}

	// parse protocol and server addr
	if !strings.Contains(addr, "://") {
		addr = "udp://" + addr
	}
	addrURL, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid server address, %w", err)
	}

	dialer := &net.Dialer{
		Control: getSocketControlFunc(socketOpts{
			so_mark:        opt.SoMark,
			bind_to_device: opt.BindToDevice,
		}),
	}

	switch addrURL.Scheme {
	case "", "udp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		uto := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialer.DialContext(ctx, "udp", dialAddr)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc: dnsutils.WriteMsgToUDP,
			ReadFunc: func(c io.Reader) (*dns.Msg, int, error) {
				return dnsutils.ReadMsgFromUDP(c, 4096)
			},
			IdleTimeout: time.Minute * 5,
		}
		tto := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialer.DialContext(ctx, "tcp", dialAddr)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc: dnsutils.WriteMsgToTCP,
			ReadFunc:  dnsutils.ReadMsgFromTCP,
		}
		return &udpWithFallback{
			u: transport.NewPipelineTransport(transport.PipelineOpts{IOOpts: uto, MaxConn: 1}),
			t: transport.NewReuseConnTransport(transport.ReuseConnOpts{IOOpts: tto}),
		}, nil
	case "tcp":
		dialAddr := getDialAddrWithPort(addrURL.Host, opt.DialAddr, 53)
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := dialTCP(ctx, dialAddr, opt.Socks5, dialer)
				c = wrapConn(c, opt.EventObserver)
				return c, err
			},
			WriteFunc:   dnsutils.WriteMsgToTCP,
			ReadFunc:    dnsutils.ReadMsgFromTCP,
			IdleTimeout: opt.IdleTimeout,
		}
		if opt.EnablePipeline {
			return transport.NewPipelineTransport(transport.PipelineOpts{IOOpts: to, MaxConn: opt.MaxConns}), nil
		}
		return transport.NewReuseConnTransport(transport.ReuseConnOpts{IOOpts: to}), nil
	default:
		return nil, fmt.Errorf("unsupported protocol [%s]", addrURL.Scheme)
	}
}

func getDialAddrWithPort(host, dialAddr string, defaultPort int) string {
	addr := host
	if len(dialAddr) > 0 {
		addr = dialAddr
	}
	_, _, err := net.SplitHostPort(addr)
	if err != nil { // no port, add it.
		return net.JoinHostPort(strings.Trim(addr, "[]"), strconv.Itoa(defaultPort))
	}
	return addr
}

func tryRemovePort(s string) string {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return s
	}
	return host
}

type udpWithFallback struct {
	u *transport.PipelineTransport
	t *transport.ReuseConnTransport
}

func (u *udpWithFallback) ExchangeContext(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	m, err := u.u.ExchangeContext(ctx, q)
	if err != nil {
		return nil, err
	}
	if m.Truncated {
		return u.t.ExchangeContext(ctx, q)
	}
	return m, nil
}

func (u *udpWithFallback) Close() error {
	u.u.Close()
	u.t.Close()
	return nil
}
