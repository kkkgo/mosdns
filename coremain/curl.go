package coremain

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

func Curl(args []string) {
	var (
		url    string
		output string
		socks5 string
	)

	if len(args) > 0 {
		url = args[0]
	}

	if len(args) > 1 {
		if strings.Contains(args[1], ":") {
			socks5 = args[1]
		} else {
			output = args[1]
		}
	}

	if len(args) > 2 {
		if strings.Contains(args[2], ":") {
			socks5 = args[2]
		} else {
			output = args[2]
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				if socks5 != "" {
					dialer, err := proxy.SOCKS5("tcp", socks5, nil, &net.Dialer{
						Timeout:   10 * time.Second,
						DualStack: false,
					})
					if err != nil {
						return nil, err
					}
					return dialer.Dial(network, addr)
				}

				return (&net.Dialer{
					Timeout:   10 * time.Second,
					DualStack: false,
				}).Dial(network, addr)
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		os.Exit(1)
	}

	req.Header.Set("User-Agent", "https://github.com/kkkgo/PaoPaoDNS")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error connecting to the server:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var outFile io.Writer
	if output != "" {
		file, err := os.Create(output)
		if err != nil {
			fmt.Println("Error creating output file:", err)
			os.Exit(1)
		}
		defer file.Close()
		outFile = file
	} else {
		outFile = os.Stdout
	}

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		fmt.Println("Error writing to output:", err)
		os.Exit(1)
	}
}
