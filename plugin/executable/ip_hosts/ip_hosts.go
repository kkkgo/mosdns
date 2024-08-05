package ip_hosts

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const PluginType = "ip_hosts"

var addInfoEnabled bool

func init() {
	addInfoEnabled = os.Getenv("ADDINFO") == "yes"

	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.RecursiveExecutable = (*IPHosts)(nil)

type IPHosts struct {
	envVarName string
	ipv4       []netip.Addr
	ipv6       []netip.Addr
}

func QuickSetup(_ sequence.BQ, s string) (any, error) {
	return NewIPHosts(s)
}

func NewIPHosts(envVarName string) (*IPHosts, error) {
	b := &IPHosts{
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
	fmt.Println("[PaoPaoDNS HOST] load:", envVarName, "=", os.Getenv(envVarName))
	return b, nil
}

func (b *IPHosts) Exec(ctx context.Context, qCtx *query_context.Context, next sequence.ChainWalker) error {
	if r := b.Response(qCtx.Q()); r != nil {
		qCtx.SetResponse(r)
	}
	// End the processing flow like ActionAccept.
	return nil
}

func (b *IPHosts) Response(q *dns.Msg) *dns.Msg {
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
					Ttl:    299,
				},
				A: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return b.addinfo(r)

	case qtype == dns.TypeAAAA && len(b.ipv6) > 0:
		r := new(dns.Msg)
		r.SetReply(q)
		for _, addr := range b.ipv6 {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qName,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    299,
				},
				AAAA: addr.AsSlice(),
			}
			r.Answer = append(r.Answer, rr)
		}
		return b.addinfo(r)
	}
	return nil
}

func (b *IPHosts) addinfo(r *dns.Msg) *dns.Msg {
	if addInfoEnabled && r != nil {
		txtRecord := new(dns.TXT)
		txtRecord.Hdr = dns.RR_Header{
			Name:   time.Now().Format("20060102150405.0000000") + ".host.paopaodns.",
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    1 + r.Answer[0].Header().Ttl,
		}
		txtRecord.Txt = []string{"Host: env_key -> " + b.envVarName}
		r.Extra = append(r.Extra, txtRecord)
	}
	return r
}
