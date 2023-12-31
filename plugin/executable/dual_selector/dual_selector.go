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

package dual_selector

import (
	"context"
	"os"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/pool"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	referenceWaitTimeout     = time.Millisecond * 400
	defaultSubRoutineTimeout = time.Millisecond * 4400
)

func init() {
	sequence.MustRegExecQuickSetup("prefer_ipv4", func(bq sequence.BQ, _ string) (any, error) {
		if os.Getenv("ADDINFO") == "yes" {
			return NewPreferIpv4(bq, true), nil
		}
		return NewPreferIpv4(bq, false), nil
	})
	sequence.MustRegExecQuickSetup("prefer_ipv6", func(bq sequence.BQ, _ string) (any, error) {
		return NewPreferIpv6(bq), nil
	})
}

var _ sequence.RecursiveExecutable = (*Selector)(nil)

type Selector struct {
	sequence.BQ
	prefer  uint16 // dns.TypeA or dns.TypeAAAA
	addinfo bool
}

// Exec implements handler.Executable.
func (s *Selector) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	q := qCtx.Q()
	if len(q.Question) != 1 { // skip wired query with multiple questions.
		return next.ExecNext(ctx, qCtx)
	}

	qtype := q.Question[0].Qtype
	// skip queries that have preferred type or have other unrelated types.
	if qtype == s.prefer || (qtype != dns.TypeA && qtype != dns.TypeAAAA) {
		return next.ExecNext(ctx, qCtx)
	}

	// start reference goroutine
	qCtxRef := qCtx.Copy()
	var refQtype uint16
	if qtype == dns.TypeA {
		refQtype = dns.TypeAAAA
	} else {
		refQtype = dns.TypeA
	}
	qCtxRef.Q().Question[0].Qtype = refQtype

	ddl, ok := ctx.Deadline()
	if !ok {
		ddl = time.Now().Add(defaultSubRoutineTimeout)
	}

	shouldBlock := make(chan struct{}, 0)
	shouldPass := make(chan struct{}, 0)
	go func() {
		qCtx := qCtxRef
		ctx, cancel := context.WithDeadline(context.Background(), ddl)
		defer cancel()
		err := next.ExecNext(ctx, qCtx)
		if err != nil {
			s.L().Warn("reference query routine err", qCtx.InfoField(), zap.Error(err))
			close(shouldPass)
			return
		}
		if r := qCtx.R(); r != nil && msgAnsHasRR(r, refQtype) {
			// Target domain has reference type.
			close(shouldBlock)
			return
		}
		close(shouldPass)
		return
	}()

	// start original query goroutine
	doneChan := make(chan error, 1)
	qCtxOrg := qCtx.Copy()
	go func() {
		qCtx := qCtxOrg
		ctx, cancel := context.WithDeadline(context.Background(), ddl)
		defer cancel()
		doneChan <- next.ExecNext(ctx, qCtx)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-shouldBlock: // Reference indicates we should block this query before the original query finished.
		if s.addinfo {
			r := BlockRecordv6(q)
			qCtx.SetResponse(r)
		} else {
			r := BlockRecord0(q)
			qCtx.SetResponse(r)
		}
		return nil
	case err := <-doneChan: // The original query finished. Waiting for reference.
		waitTimeoutTimer := pool.GetTimer(referenceWaitTimeout)
		defer pool.ReleaseTimer(waitTimeoutTimer)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-shouldBlock:
			if s.addinfo {
				r := BlockRecordv6(q)
				qCtx.SetResponse(r)
			} else {
				r := BlockRecord0(q)
				qCtx.SetResponse(r)
			}
			return nil
		case <-shouldPass:
			*qCtx = *qCtxOrg // replace qCtx
			return err
		case <-waitTimeoutTimer.C:
			// We have been waiting the reference query for too long.
			// Something may go wrong. We accept the original reply.
			*qCtx = *qCtxOrg
			return err
		}
	}
}

func BlockRecord0(q *dns.Msg) *dns.Msg {
	r := new(dns.Msg)
	r.SetRcode(q, 0)
	r.Answer = []dns.RR{}
	return r
}

func BlockRecordv6(q *dns.Msg) *dns.Msg {
	r := new(dns.Msg)
	r.SetRcode(q, 0)
	r.Answer = []dns.RR{}
	txtRecord := new(dns.TXT)
	txtRecord.Hdr = dns.RR_Header{
		Name:   time.Now().Format("20060102150405.000") + ".block.paopaodns.",
		Rrtype: dns.TypeTXT,
		Class:  dns.ClassINET,
		Ttl:    0,
	}
	txtRecord.Txt = []string{"Records may exist, but blocked by PaoPaoDNS IPV6 option."}
	r.Extra = append(r.Extra, txtRecord)
	return r
}

func NewPreferIpv4(bq sequence.BQ, addinfo bool) *Selector {
	return &Selector{
		BQ:      bq,
		prefer:  dns.TypeA,
		addinfo: addinfo,
	}
}

func NewPreferIpv6(bq sequence.BQ) *Selector {
	return &Selector{
		BQ:      bq,
		prefer:  dns.TypeAAAA,
		addinfo: false,
	}
}

func msgAnsHasRR(m *dns.Msg, t uint16) bool {
	if len(m.Answer) == 0 {
		return false
	}

	for _, rr := range m.Answer {
		if rr.Header().Rrtype == t {
			return true
		}
	}
	return false
}
