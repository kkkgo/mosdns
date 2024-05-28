package flushd_server

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

const (
	sockPath      = "/tmp/flush.sock"
	unboundSocket = "/tmp/uc_raw.sock"
	dnsServer     = "127.0.0.1:5301"
	ubctPrefix    = "UBCT1 "
)

type request struct {
	domain string
	timer  *time.Timer
}

var (
	requestQueue = make(chan request, 100)
	wg           sync.WaitGroup
)

func handleRequest(r request, logger *zap.Logger) {
	defer wg.Done()

	domain := r.domain

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	resp, err := dns.Exchange(m, dnsServer)
	if err != nil {
		logger.Warn("Error querying DNS", zap.String("domain", domain), zap.Error(err))
		return
	}

	if len(resp.Answer) > 0 {
		logger.Info("DNS response", zap.String("domain", domain), zap.Int("TTL", int(resp.Answer[0].Header().Ttl)))

		if resp.Answer[0].Header().Ttl == 0 {
			flushCache(domain, logger)

			resp, err = dns.Exchange(m, dnsServer)
			if err != nil {
				logger.Warn("Error querying DNS after flush", zap.String("domain", domain), zap.Error(err))
				return
			}

			logger.Info("DNS response after flush", zap.String("domain", domain), zap.Int("TTL", int(resp.Answer[0].Header().Ttl)))
		} else {
			logger.Info("No need to flush cache", zap.String("domain", domain))
		}
	} else {
		logger.Info("No valid DNS response", zap.String("domain", domain))
	}
}

func flushCache(domain string, logger *zap.Logger) {
	conn, err := net.Dial("unix", unboundSocket)
	if err != nil {
		logger.Warn("Error connecting to unbound socket", zap.Error(err))
		return
	}
	defer conn.Close()

	cmd := fmt.Sprintf("flush +c %s", domain)
	_, err = fmt.Fprintf(conn, "%s%s\n", ubctPrefix, cmd)
	if err != nil {
		logger.Warn("Error sending flush command", zap.String("domain", domain), zap.Error(err))
		return
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.HasPrefix(line, "error") {
			logger.Warn("Error response from unbound", zap.String("response", line))
			break
		} else {
			logger.Debug("Response from unbound", zap.String("response", line))
		}
	}
}

func handleConnection(conn net.Conn, logger *zap.Logger) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	domain, err := reader.ReadString('\n')
	if err != nil {
		logger.Warn("Error reading domain from client", zap.Error(err))
		return
	}

	domain = strings.TrimSpace(domain)
	timer := time.NewTimer(4 * time.Second)
	requestQueue <- request{domain, timer}
	logger.Info("Received request for domain", zap.String("domain", domain))
}

func processQueue(logger *zap.Logger) {
	for r := range requestQueue {
		<-r.timer.C
		wg.Add(1)
		go handleRequest(r, logger)
	}
}

func Init(bp *coremain.BP, args any) (any, error) {
	logger := bp.L()

	os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		logger.Warn("Error creating unix socket listener", zap.Error(err))
		return nil, err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				logger.Warn("Error accepting unix connection", zap.Error(err))
				continue
			}

			go handleConnection(conn, logger)
		}
	}()

	go processQueue(logger)
	fmt.Println("flushd server start on path", sockPath, "...")

	return nil, nil
}

func init() {
	coremain.RegNewPluginFunc("flushd_server", Init, func() any { return new(struct{}) })
}
