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

package sequence

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/miekg/dns"
)

var _ RecursiveExecutable = (*ActionAccept)(nil)

type ActionAccept struct{}

func (a ActionAccept) Exec(_ context.Context, _ *query_context.Context, _ ChainWalker) error {
	return nil
}

func setupAccept(_ BQ, _ string) (any, error) {
	return ActionAccept{}, nil
}

var _ RecursiveExecutable = (*ActionReject)(nil)

type ActionReject struct {
	Rcode int
}

func (a ActionReject) Exec(_ context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	r.Rcode = a.Rcode
	qCtx.SetResponse(r)
	return nil
}

func setupReject(_ BQ, s string) (any, error) {
	rcode := 0
	if len(s) > 0 {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 0xFFF {
			return nil, fmt.Errorf("invalid rcode [%s]", s)
		}
		rcode = n
	}
	return ActionReject{Rcode: rcode}, nil
}

var _ RecursiveExecutable = (*ActionPong)(nil)

type ActionPong struct {
	DebugInfo string
	AllowErr  bool
}

func (a ActionPong) Exec(_ context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	r := new(dns.Msg)
	r.SetReply(qCtx.Q())
	if a.AllowErr && qCtx.R() != nil {
		r.Rcode = qCtx.R().Rcode
	} else {
		r.Rcode = 0
	}
	r.Authoritative = true
	r.RecursionAvailable = true
	r.Answer = []dns.RR{}
	if a.DebugInfo != "" {
		if qCtx != nil {
			query_time := "nil"
			query_time = fmt.Sprintf("%dms", time.Since(qCtx.StartTime()).Milliseconds())
			txtRecord := new(dns.TXT)
			txtRecord.Hdr = dns.RR_Header{
				Name:   time.Now().Format("20060102150405.000") + ".reject.paopaodns.",
				Rrtype: dns.TypeTXT,
				Class:  dns.ClassINET,
				Ttl:    0,
			}
			txtRecord.Txt = []string{query_time + ", " + a.DebugInfo}
			r.Extra = []dns.RR{txtRecord}
		}
	}
	qCtx.SetResponse(r)
	return nil
}

func setupPong(_ BQ, s string) (any, error) {
	if os.Getenv("ADDINFO") == "yes" {
		return ActionPong{DebugInfo: s, AllowErr: false}, nil
	}
	return ActionPong{DebugInfo: "", AllowErr: false}, nil
}

func setupPongerr(_ BQ, s string) (any, error) {
	if os.Getenv("ADDINFO") == "yes" {
		return ActionPong{DebugInfo: s, AllowErr: true}, nil
	}
	return ActionPong{DebugInfo: "", AllowErr: true}, nil
}

var _ RecursiveExecutable = (*ActionReturn)(nil)

type ActionReturn struct{}

func (a ActionReturn) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	if next.jumpBack != nil {
		return next.jumpBack.ExecNext(ctx, qCtx)
	}
	return nil
}

func setupReturn(_ BQ, _ string) (any, error) {
	return ActionReturn{}, nil
}

var _ RecursiveExecutable = (*ActionJump)(nil)

type ActionJump struct {
	To []*ChainNode
}

func (a *ActionJump) Exec(ctx context.Context, qCtx *query_context.Context, next ChainWalker) error {
	w := NewChainWalker(a.To, &next)
	return w.ExecNext(ctx, qCtx)
}

func setupJump(bq BQ, s string) (any, error) {
	target, _ := bq.M().GetPlugin(s).(*Sequence)
	if target == nil {
		return nil, fmt.Errorf("can not find jump target %s", s)
	}
	return &ActionJump{To: target.chain}, nil
}

var _ RecursiveExecutable = (*ActionGoto)(nil)

type ActionGoto struct {
	To []*ChainNode
}

func (a ActionGoto) Exec(ctx context.Context, qCtx *query_context.Context, _ ChainWalker) error {
	w := NewChainWalker(a.To, nil)
	return w.ExecNext(ctx, qCtx)
}

func setupGoto(bq BQ, s string) (any, error) {
	gt, _ := bq.M().GetPlugin(s).(*Sequence)
	if gt == nil {
		return nil, fmt.Errorf("can not find goto target %s", s)
	}
	return &ActionGoto{To: gt.chain}, nil
}

var _ Matcher = (*MatchAlwaysTrue)(nil)

type MatchAlwaysTrue struct{}

func (m MatchAlwaysTrue) Match(_ context.Context, _ *query_context.Context) (bool, error) {
	return true, nil
}

func setupTrue(_ BQ, _ string) (Matcher, error) {
	return MatchAlwaysTrue{}, nil
}

var _ Matcher = (*MatchAlwaysFalse)(nil)

type MatchAlwaysFalse struct{}

func (m MatchAlwaysFalse) Match(_ context.Context, _ *query_context.Context) (bool, error) {
	return false, nil
}

func setupFalse(_ BQ, _ string) (Matcher, error) {
	return MatchAlwaysFalse{}, nil
}
