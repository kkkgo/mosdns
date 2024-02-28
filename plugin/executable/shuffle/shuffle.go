// PaoPaoDNS shuffle with lite mode

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

type Shuffle struct {
    lite bool
}

func NewShuffle(lite bool) *Shuffle {
    return &Shuffle{lite: lite}
}

func QuickSetup(_ sequence.BQ, s string) (interface{}, error) {
    lite := s == "lite"
    return NewShuffle(lite), nil
}

func (s *Shuffle) Exec(_ context.Context, qCtx *query_context.Context) error {
    response := qCtx.R()
    request := qCtx.Q()

    if response == nil || response.Answer == nil {
        return nil
    }

    if s.lite {
        filteredAnswers :=  FilterType(response.Answer, request.Question[0].Qtype)
		ShuffleRecord(filteredAnswers)
        response.Answer = filteredAnswers
    } else {
         ShuffleSkipCNAME(response.Answer)
    }

    return nil
}

func  FilterType(answers []dns.RR, qtype uint16) []dns.RR {
    var filtered []dns.RR
    for _, answer := range answers {
        if answer.Header().Rrtype == qtype {
            filtered = append(filtered, answer)
        }
    }
    return filtered
}

func  ShuffleRecord(answers []dns.RR) {
    n := len(answers)
    for i := n - 1; i > 0; i-- {
        j := rand.Intn(i + 1)
        answers[i], answers[j] = answers[j], answers[i]
    }
}

func  ShuffleSkipCNAME(answers []dns.RR) {
    n := len(answers)
    for i := 0; i < n; i++ {
        if _, isCNAME := answers[i].(*dns.CNAME); isCNAME {
            continue
        }
        for j := i + 1; j < n; j++ {
            if _, isCNAME := answers[j].(*dns.CNAME); !isCNAME {
                randIndex := rand.Intn(j-i+1) + i
                answers[i], answers[randIndex] = answers[randIndex], answers[i]
                break
            }
        }
    }
}
