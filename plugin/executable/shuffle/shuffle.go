// PaoPaoDNS shuffle

package shuffle

import (
	"context"
	"math/rand"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
)

const (
	PluginType = "shuffle"
)

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*Shuffle)(nil)

type Shuffle struct {
}

func NewShuffle() *Shuffle {
	return &Shuffle{}
}

func QuickSetup(_ sequence.BQ, s string) (interface{}, error) {
	return NewShuffle(), nil
}

func (s *Shuffle) Exec(_ context.Context, qCtx *query_context.Context) error {
	response := qCtx.R()
	if response == nil || response.Answer == nil {
		return nil
	}

	ShuffleDNSAnswers(response)
	MoveCNAMEToFirst(response)
	return nil
}

func ShuffleDNSAnswers(response *dns.Msg) {
	answers := response.Answer
	n := len(answers)
	for i := n - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		answers[i], answers[j] = answers[j], answers[i]
	}
}

func MoveCNAMEToFirst(response *dns.Msg) {
	answers := response.Answer
	cnameRecords := make([]dns.RR, 0)
	nonCnameRecords := make([]dns.RR, 0)

	for _, record := range answers {
		if _, isCNAME := record.(*dns.CNAME); isCNAME {
			cnameRecords = append(cnameRecords, record)
		} else {
			nonCnameRecords = append(nonCnameRecords, record)
		}
	}
	answers = append(cnameRecords, nonCnameRecords...)
	response.Answer = answers
}
