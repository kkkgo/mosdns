// PaoPaoDNS addinfo
package addinfo

import (
	"context"
	"fmt"
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
	if r := qCtx.R(); r != nil && qCtx.R().Answer != nil {
		var ttl uint32 = 600
		if len(qCtx.R().Answer) > 0 {
			ttl = qCtx.R().Answer[0].Header().Ttl + 1
		}
		query_time := fmt.Sprintf("Since %dms ", time.Since(qCtx.StartTime()).Milliseconds())
		txtRecord := new(dns.TXT)
		txtRecord.Hdr = dns.RR_Header{
			Name:   time.Now().Format("20060102150405.000000000") + ".addinfo.paopaodns.",
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    ttl,
		}
		txtRecord.Txt = []string{query_time + "From:" + t.txtRecord}

		r.Extra = append(r.Extra, txtRecord)
	}
	return nil
}
