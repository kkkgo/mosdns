// PaoPaoDNS addinfo
package addinfo

import (
	"context"
	"time"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const (
	PluginType = "addinfo"
)

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*addinfo)(nil)

type addinfo struct {
	txtRecord string
}

func Newaddinfo(txtRecord string) *addinfo {
	return &addinfo{
		txtRecord: txtRecord,
	}
}

func QuickSetup(_ sequence.BQ, s string) (any, error) {
	return Newaddinfo(s), nil
}

func (t *addinfo) Exec(_ context.Context, qCtx *query_context.Context) error {
	if r := qCtx.R(); r != nil {
		txtRecord := new(dns.TXT)
		txtRecord.Hdr = dns.RR_Header{
			Name:   time.Now().Format("20060102150405.0000000") + ".addinfo.paopaodns.",
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    0,
		}
		txtRecord.Txt = []string{", Respond from:" + t.txtRecord}

		r.Extra = append(r.Extra, txtRecord)
	}
	return nil
}
