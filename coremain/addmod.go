package coremain

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type ZoneConfig struct {
	Zone   string
	DNS    string
	Socks5 string
	TTL    int
}

type SwapsConfig struct {
	Env_key   string
	Cidr_file string
}

type AddModConfig struct {
	Zones []ZoneConfig
	Swaps []SwapsConfig
}

var allcontent string

func AddMod() {
	viper.SetConfigFile("/data/custom_mod.yaml")
	err := viper.ReadInConfig()
	if err != nil {
		fmt.Printf("Unable Load Custom_mod: %s\n", err)
		return
	}
	var config AddModConfig
	err = viper.Unmarshal(&config)
	if err != nil {
		fmt.Printf("Error unmarshaling Custom_mod: %s\n", err)
		return
	}
	// fmt.Println(config.Zones)
	// fmt.Println(config.Swaps)

	filePath := "/tmp/mosdns.yaml"
	if err := readConfigFile(filePath); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	//get yaml config.
	f, s, q := genZones(config.Zones)
	i, r := genSwaps(config.Swaps)
	insertAfterKeyStart("forward_zones_dns_start", f)
	insertAfterKeyStart("forward_zones_seq_start", s)
	insertAfterKeyStart("forward_zones_qname_start", q)
	insertAfterKeyStart("forward_swaps_ipset_start", i)
	insertAfterKeyStart("forward_swaps_match1_start", r)
	insertAfterKeyStart("forward_swaps_match2_start", r)
	// fmt.Println(allcontent)

	outputFilePath := "/tmp/mosdns_mod.yaml"
	if err := writeToFile(outputFilePath); err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
	}
}

// return forward, seq, match
func genZones(zones []ZoneConfig) (string, string, string) {
	var forwardText strings.Builder
	var sequenceText strings.Builder
	var qnameText strings.Builder

	for _, zone := range zones {
		if zone.DNS == "" {
			continue
		}

		//gen forward
		dnsAddresses := strings.Split(zone.DNS, ",")

		var upstreamsText strings.Builder
		for _, dnsAddress := range dnsAddresses {
			socks5Option := ""
			if strings.HasPrefix(dnsAddress, "tcp://") && zone.Socks5 == "yes" {
				if os.Getenv("SOCKS5") != "" {
					socks5Option = fmt.Sprintf("          socks5: \"%s\"\n", os.Getenv("SOCKS5"))
				}
			}

			upstreamsText.WriteString(fmt.Sprintf("        - addr: \"%s\"\n", dnsAddress))
			upstreamsText.WriteString(socks5Option)
		}

		forwardText.WriteString(fmt.Sprintf(`  - tag: forward_zones@%s
    type: forward
    args:
      concurrent: 3
      upstreams:
%s`, zone.Zone, upstreamsText.String()))

		forwardText.WriteString("\n")

		//gen seq
		sequenceText.WriteString(fmt.Sprintf(`  - tag: sequence@%s
    type: sequence
    args:
        - exec: $forward_zones@%s
`, zone.Zone, zone.Zone))
		if zone.TTL > 0 {
			sequenceText.WriteString(fmt.Sprintf(`        - exec: ttl 0-%d
`, zone.TTL))
		}
		sequenceText.WriteString(fmt.Sprintf(`        - exec: respond forward_zones@%s
`, zone.Zone))

		//gen qname match
		qnameText.WriteString(fmt.Sprintf(`        - matches: qname domain:%s
          exec: goto sequence@%s
`, zone.Zone, zone.Zone))
	}

	return forwardText.String(), sequenceText.String(), qnameText.String()
}

// return ip_set, match
func genSwaps(swaps []SwapsConfig) (string, string) {
	var ipsetText strings.Builder
	var rewriteText strings.Builder

	for _, swap := range swaps {
		if swap.Env_key == "" || swap.Cidr_file == "" {
			continue
		}

		ipsetText.WriteString(fmt.Sprintf(`  - tag: ip_set@%s
    type: ip_set
    args:
        - files: "%s"
`, swap.Env_key, swap.Cidr_file))

		//gen resp match
		rewriteText.WriteString(fmt.Sprintf(`        - matches: resp_ip $ip_set@%s
          exec: ip_rewrite %s
`, swap.Env_key, swap.Env_key))
	}
	return ipsetText.String(), rewriteText.String()
}

func readConfigFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allcontent += scanner.Text() + "\n"
	}

	if scanner.Err() != nil {
		return scanner.Err()
	}

	return nil
}

func insertAfterKeyStart(keystart, content string) {
	lines := strings.Split(allcontent, "\n")
	for i, line := range lines {
		if strings.Contains(line, keystart) {
			lines = append(lines[:i+1], append([]string{content}, lines[i+1:]...)...)
			break
		}
	}
	allcontent = strings.Join(lines, "\n")
}

func writeToFile(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	_, err = writer.WriteString(allcontent)
	if err != nil {
		return err
	}

	err = writer.Flush()
	if err != nil {
		return err
	}

	return nil
}
