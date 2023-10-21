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
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/mlog"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/upstream/transport"
	"github.com/miekg/dns"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
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
	// actually dial to in the network layer by overwriting
	// the address inferred from upstream url.
	// It won't affect high level layers. (e.g. SNI, HTTP HOST header won't be changed).
	// Can be an IP or a domain. Port is optional.
	// Tips: If the upstream url host is a domain, specific an IP address
	// here can skip resolving ip of this domain.
	DialAddr string

	// Socks5 specifies the socks5 proxy server that the upstream
	// will connect though.
	// Not implemented for udp based protocols (aka. dns over udp, http3, quic).
	Socks5 string

	// SoMark sets the socket SO_MARK option in unix system.
	SoMark int

	// BindToDevice sets the socket SO_BINDTODEVICE option in unix system.
	BindToDevice string

	// IdleTimeout specifies the idle timeout for long-connections.
	// Available for TCP, DoT, DoH.
	// Default: TCP, DoT: 10s , DoH, DoQ: 30s.
	IdleTimeout time.Duration

	// EnablePipeline enables query pipelining support as RFC 7766 6.2.1.1 suggested.
	// Available for TCP, DoT upstream with IdleTimeout >= 0.
	// Note: There is no fallback.
	EnablePipeline bool

	// MaxConns limits the total number of connections, including connections
	// in the dialing states.
	// Implemented for TCP/DoT pipeline enabled upstream and DoH upstream.
	// Default is 2.
	MaxConns int

	// Logger specifies the logger that the upstream will use.
	Logger *zap.Logger

	// EventObserver can observe connection events.
	// Not implemented for udp based protocols (dns over udp, http3, quic).
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

	// If host is a ipv6 without port, it will be in []. This will cause err when
	// split and join address and port. Try to remove brackets now.
	addrUrlHost := tryTrimIpv6Brackets(addrURL.Host)

	dialer := &net.Dialer{
		Control: getSocketControlFunc(socketOpts{
			so_mark:        opt.SoMark,
			bind_to_device: opt.BindToDevice,
		}),
	}

	newTcpDialer := func(dialAddrMustBeIp bool, defaultPort uint16) (func(ctx context.Context) (net.Conn, error), error) {
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}

		// Socks5 enabled.
		if s5Addr := opt.Socks5; len(s5Addr) > 0 {
			socks5Dialer, err := proxy.SOCKS5("tcp", s5Addr, nil, dialer)
			if err != nil {
				return nil, fmt.Errorf("failed to init socks5 dialer: %w", err)
			}

			contextDialer := socks5Dialer.(proxy.ContextDialer)
			dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			return func(ctx context.Context) (net.Conn, error) {
				return contextDialer.DialContext(ctx, "tcp", dialAddr)
			}, nil
		}

		if _, err := netip.ParseAddr(host); err == nil {
			// Host is an ip addr. No need to resolve it.
			dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			return func(ctx context.Context) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", dialAddr)
			}, nil
		} else {
			if dialAddrMustBeIp {
				return nil, errors.New("addr must be an ip address")
			}
			// Host is not an ip addr, assuming it is a domain.
			dialAddr := net.JoinHostPort(host, strconv.Itoa(int(port)))
			return func(ctx context.Context) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp", dialAddr)
			}, nil
		}
	}

	switch addrURL.Scheme {
	case "", "udp":
		const defaultPort = 53
		host, port, err := parseDialAddr(addrUrlHost, opt.DialAddr, defaultPort)
		if err != nil {
			return nil, err
		}
		if _, err := netip.ParseAddr(host); err != nil {
			return nil, fmt.Errorf("addr must be an ip address, %w", err)
		}
		dialAddr := joinPort(host, port)
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
		const defaultPort = 53
		tcpDialer, err := newTcpDialer(true, defaultPort)
		if err != nil {
			return nil, fmt.Errorf("failed to init tcp dialer, %w", err)
		}
		to := transport.IOOpts{
			DialFunc: func(ctx context.Context) (io.ReadWriteCloser, error) {
				c, err := tcpDialer(ctx)
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
