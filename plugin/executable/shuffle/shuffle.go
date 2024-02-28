// PaoPaoDNS shuffle with lite modes

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
    mode int
}

func NewShuffle(mode int) *Shuffle {
    return &Shuffle{mode: mode}
}

func QuickSetup(_ sequence.BQ, s string) (interface{}, error) {
    mode := 0
    switch s {
    case "1":
        mode = 1
    case "2":
        mode = 2
    case "3":
        mode = 3
    }
    return NewShuffle(mode), nil
}

func (s *Shuffle) Exec(_ context.Context, qCtx *query_context.Context) error {
    response := qCtx.R()
    request := qCtx.Q()

    if response == nil || response.Answer == nil {
        return nil
    }

    switch s.mode {
    case 1: //filter and shuffle
        filteredAnswers := FilterType(response.Answer, request.Question[0].Qtype)
        ShuffleRecord(filteredAnswers)
        response.Answer = filteredAnswers
    case 2: //filter
        filteredAnswers := FilterType(response.Answer, request.Question[0].Qtype)
        response.Answer = filteredAnswers
    case 3: //shuffle
        ShuffleRecord(response.Answer)
    default: //shuffle but not shuffle cname
        ShuffleSkipCNAME(response.Answer)
    }

    return nil
}

func FilterType(answers []dns.RR, qtype uint16) []dns.RR {
    var filtered []dns.RR
    for _, answer := range answers {
        if answer.Header().Rrtype == qtype {
            filtered = append(filtered, answer)
        }
    }
    return filtered
}

func ShuffleRecord(answers []dns.RR) {
    n := len(answers)
    for i := n - 1; i > 0; i-- {
        j := rand.Intn(i + 1)
        answers[i], answers[j] = answers[j], answers[i]
    }
}

func ShuffleSkipCNAME(answers []dns.RR) {
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
