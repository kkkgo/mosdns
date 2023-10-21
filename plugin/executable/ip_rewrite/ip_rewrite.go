package ip_rewrite

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "ip_rewrite"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*IPRewrite)(nil)

type IPRewrite struct {
	envVarName string
	ipv4       []netip.Addr
	ipv6       []netip.Addr
}

func QuickSetup(_ sequence.BQ, s string) (any, error) {
	return NewIPRewrite(s)
}

func NewIPRewrite(envVarName string) (*IPRewrite, error) {
	b := &IPRewrite{
		envVarName: envVarName,
	}

	addresses := os.Getenv(envVarName)
	if addresses == "" {
		fmt.Println("[PaoPaoDNS ERROR] env_key is not set: ", envVarName+"=?")
		return nil, nil
	}

	ips := strings.Fields(addresses)
	for _, s := range ips {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			fmt.Println("[PaoPaoDNS ERROR] invalid address: ", s, ", debug:", err)
			return nil, nil
		}
		if addr.Is4() {
			b.ipv4 = append(b.ipv4, addr)
		} else {
			b.ipv6 = append(b.ipv6, addr)
		}
	}
	return b, nil
}

func (b *IPRewrite) Exec(_ context.Context, qCtx *query_context.Context) error {
	if r := b.Response(qCtx.Q()); r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}

func (b *IPRewrite) Response(q *dns.Msg) *dns.Msg {
	if len(q.Question) != 1 {
		return nil
	}

	qName := q.Question[0].Name
	qtype := q.Question[0].Qtype

	switch {
	case qtype == dns.TypeA && len(b.ipv4) > 0:
		r := new(dns.Msg)
		r.SetReply(q)
		for _, addr := range b.ipv4 {
			rr := &dns.A{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				A: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return r

	case qtype == dns.TypeAAAA && len(b.ipv6) > 0:
		r := new(dns.Msg)
		r.SetReply(q)
		for _, addr := range b.ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    60,
				},
				AAAA: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return r
	}
	return nil
}