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

	"github.com/IrineSistiana/mosdns/v5/pkg/upstream"
	"github.com/miekg/dns"
	"go.uber.org/zap/zapcore"
)

type upstreamWrapper struct {
	idx int
	u   upstream.Upstream
	cfg UpstreamConfig
}

// newWrapper inits all metrics.
// Note: upstreamWrapper.u still needs to be set.
func newWrapper(idx int, cfg UpstreamConfig, pluginTag string) *upstreamWrapper {
	return &upstreamWrapper{
		cfg: cfg,
	}
}

// name returns upstream tag if it was set in the config.
// Otherwise, it returns upstream address.
func (uw *upstreamWrapper) name() string {
	if t := uw.cfg.Tag; len(t) > 0 {
		return uw.cfg.Tag
	}
	return uw.cfg.Addr
}

func (uw *upstreamWrapper) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	r, err := uw.u.ExchangeContext(ctx, m)
	return r, err
}

func (uw *upstreamWrapper) Close() error {
	return uw.u.Close()
}

type queryInfo dns.Msg

func (q *queryInfo) MarshalLogObject(encoder zapcore.ObjectEncoder) error {
	if len(q.Question) != 1 {
		encoder.AddBool("odd_question", true)
	} else {
		question := q.Question[0]
		encoder.AddString("qname", question.Name)
		encoder.AddUint16("qtype", question.Qtype)
		encoder.AddUint16("qclass", question.Qclass)
	}
	return nil
}
