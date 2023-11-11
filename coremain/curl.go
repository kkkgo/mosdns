package coremain

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/proxy"
)

func Curl(args []string) {
	url := args[0]
	output := args[1]
	socks5 := args[2]
	if url == "" {
		fmt.Println("Please provide a URL using url parameter.")
		os.Exit(1)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				if socks5 != "no" {
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

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error connecting to the server:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var outFile io.Writer
	if output != "s" {
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
