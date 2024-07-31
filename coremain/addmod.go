package coremain

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

type ModConfig struct {
	Zones []Zone `mapstructure:"Zones"`
	Swaps []Swap `mapstructure:"Swaps"`
}

type Zone struct {
	Zone   string `mapstructure:"zone"`
	DNS    string `mapstructure:"dns"`
	TTL    int    `mapstructure:"ttl"`
	Seq    string `mapstructure:"seq"`
	Socks5 string `mapstructure:"socks5"`
}

type Swap struct {
	EnvKey   string `mapstructure:"env_key"`
	CIDRFile string `mapstructure:"cidr_file"`
}

func AddMod() {
	v := viper.New()
	v.SetConfigFile("/data/custom_mod.yaml")
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		fmt.Println("Error reading config file:", err)
		return
	}

	var config ModConfig
	if err := v.Unmarshal(&config); err != nil {
		fmt.Println("Error parsing config file:", err)
		return
	}

	templateData, err := os.ReadFile("/tmp/mosdns.yaml")
	if err != nil {
		fmt.Println("Error reading template file:", err)
		return
	}

	template := string(templateData)

	forwardPlugins := make(map[string]string)
	sequencePlugins := make(map[string]string)
	zoneMatches := make(map[string][]Zone)
	forwardCount := 0
	sequenceCount := 0
	var socks5Pattern = regexp.MustCompile(`^.+:[0-9]+$`)

	for _, zone := range config.Zones {
		socks5Value := ""
		if zone.Socks5 == "yes" {
			socks5Value = os.Getenv("SOCKS5")
			if socks5Value == "" || !socks5Pattern.MatchString(socks5Value) {
				fmt.Println("[PaoPaoDNS ZONE]! SOCKS5 not found or invalid, skipping zone:", zone.Zone)
				continue
			}
		}

		forwardKey := fmt.Sprintf("%s%s", zone.DNS, socks5Value)
		sequenceKey := fmt.Sprintf("%s%s%d", zone.DNS, socks5Value, zone.TTL)

		var forwardTag string
		if tag, exists := forwardPlugins[forwardKey]; exists {
			forwardTag = tag
		} else {
			forwardCount++
			forwardTag = fmt.Sprintf("forward_zones@%d", forwardCount)
			forwardPlugins[forwardKey] = forwardTag
		}

		var sequenceTag string
		if tag, exists := sequencePlugins[sequenceKey]; exists {
			sequenceTag = tag
		} else {
			sequenceCount++
			sequenceTag = fmt.Sprintf("sequence@%d", sequenceCount)
			sequencePlugins[sequenceKey] = sequenceTag
		}

		zoneMatches[sequenceTag] = append(zoneMatches[sequenceTag], zone)
	}

	var forwardConfig strings.Builder
	for forwardKey, forwardTag := range forwardPlugins {
		forwardConfig.WriteString(generateForwardPlugin(forwardTag, forwardKey, config.Zones))
	}
	template = strings.Replace(template, "##zones_dns_start##\n##zones_dns_end##", "##zones_dns_start##\n"+forwardConfig.String()+"##zones_dns_end##", 1)

	var sequenceConfig strings.Builder
	for sequenceKey, sequenceTag := range sequencePlugins {
		sequenceConfig.WriteString(generateSequencePlugin(sequenceTag, sequenceKey, config.Zones, forwardPlugins))
	}
	template = strings.Replace(template, "##zones_seq_start##\n##zones_seq_end##", "##zones_seq_start##\n"+sequenceConfig.String()+"##zones_seq_end##", 1)

	var topConfig, top6Config, listConfig strings.Builder
	for sequenceTag, zones := range zoneMatches {
		var zoneNames []string
		for _, zone := range zones {
			zoneName := zone.Zone
			if !strings.Contains(zoneName, ":") {
				zoneName = "domain:" + zoneName
			}
			zoneNames = append(zoneNames, zoneName)
		}
		matchConfig := fmt.Sprintf("        - matches: qname %s\n          exec: goto %s\n", strings.Join(zoneNames, " "), sequenceTag)
		switch zones[0].Seq {
		case "top6":
			top6Config.WriteString(matchConfig)
		case "list":
			listConfig.WriteString(matchConfig)
		default:
			topConfig.WriteString(matchConfig)
		}
	}
	template = strings.Replace(template, "##zones_qname_top_start##\n##zones_qname_top_end##", "##zones_qname_top_start##\n"+topConfig.String()+"##zones_qname_top_end##", 1)
	template = strings.Replace(template, "##zones_qname_top6_start##\n##zones_qname_top6_end##", "##zones_qname_top6_start##\n"+top6Config.String()+"##zones_qname_top6_end##", 1)
	template = strings.Replace(template, "##zones_qname_list_start##\n##zones_qname_list_end##", "##zones_qname_list_start##\n"+listConfig.String()+"##zones_qname_list_end##", 1)

	envKeyToCIDRFiles := make(map[string][]string)
	seenCIDRFiles := make(map[string]bool)
	for _, swap := range config.Swaps {
		if _, err := os.Stat(swap.CIDRFile); os.IsNotExist(err) {
			fmt.Printf("[PaoPaoDNS SWAP]! CIDR file not found: %s\n", swap.CIDRFile)
			continue
		}
		if envValue := os.Getenv(swap.EnvKey); envValue != "" {
			if seenCIDRFiles[swap.CIDRFile] {
				fmt.Printf("[PaoPaoDNS SWAP]! CIDR file %s is already matched to an env_key, skipping\n", swap.CIDRFile)
				continue
			}
			envKeyToCIDRFiles[swap.EnvKey] = append(envKeyToCIDRFiles[swap.EnvKey], swap.CIDRFile)
			seenCIDRFiles[swap.CIDRFile] = true
			fmt.Printf("[PaoPaoDNS SWAP] get: %s = %s\n", swap.EnvKey, envValue)
		} else {
			fmt.Printf("[PaoPaoDNS SWAP]! Env key not found or empty: %s\n", swap.EnvKey)
		}
	}

	var matchConfig strings.Builder
	for envKey, cidrFiles := range envKeyToCIDRFiles {
		if len(cidrFiles) > 1 {
			matchConfig.WriteString(fmt.Sprintf("        - matches: resp_ip &%s\n          exec: ip_rewrite %s\n", strings.Join(cidrFiles, " &"), envKey))
		} else {
			matchConfig.WriteString(fmt.Sprintf("        - matches: resp_ip &%s\n          exec: ip_rewrite %s\n", cidrFiles[0], envKey))
		}
	}
	template = strings.Replace(template, "##swaps_match_start##\n##swaps_match_end##", "##swaps_match_start##\n"+matchConfig.String()+"##swaps_match_end##", 1)

	err = os.WriteFile("/tmp/mosdns_mod.yaml", []byte(template), 0644)
	if err != nil {
		fmt.Println("Error writing output file:", err)
		return
	}

	fmt.Println("[PaoPaoDNS ADDMOD] Configuration generated.")
}

func generateForwardPlugin(tag, key string, zones []Zone) string {
	var zone Zone
	var socks5Value string
	var socks5Pattern = regexp.MustCompile(`^.+:[0-9]+$`)
	for _, z := range zones {
		if z.Socks5 == "yes" {
			socks5Value = os.Getenv("SOCKS5")
			if socks5Value == "" || !socks5Pattern.MatchString(socks5Value) {
				continue
			}
		}
		if fmt.Sprintf("%s%s", z.DNS, socks5Value) == key {
			zone = z
			break
		}
	}

	var socks5Config string
	if socks5Value != "" {
		socks5Config = fmt.Sprintf("      socks5: %s\n", socks5Value)
	}

	return fmt.Sprintf(`  - tag: %s
    type: forward
    args:
      concurrent: 3
      allowcode: 23
%s      upstreams:
%s
`, tag, socks5Config, generateUpstreams(zone.DNS))
}

func generateUpstreams(dns string) string {
	var upstreams strings.Builder
	for _, addr := range strings.Split(dns, ",") {
		upstreams.WriteString(fmt.Sprintf("        - addr: \"%s\"\n", addr))
	}
	return upstreams.String()
}

func generateSequencePlugin(tag, key string, zones []Zone, forwardPlugins map[string]string) string {
	var zone Zone
	var socks5Value string
	var socks5Pattern = regexp.MustCompile(`^.+:[0-9]+$`)

	for _, z := range zones {
		if z.Socks5 == "yes" {
			socks5Value = os.Getenv("SOCKS5")
			if socks5Value == "" || !socks5Pattern.MatchString(socks5Value) {
				continue
			}
		}
		if fmt.Sprintf("%s%s%d", z.DNS, socks5Value, z.TTL) == key {
			zone = z
			break
		}
	}

	ttlConfig := ""
	if zone.TTL > 0 {
		ttlConfig = fmt.Sprintf("        - exec: ttl 0-%d\n", zone.TTL)
	}

	forwardKey := fmt.Sprintf("%s%s", zone.DNS, socks5Value)
	forwardTag := forwardPlugins[forwardKey]

	return fmt.Sprintf(`  - tag: %s
    type: sequence
    args:
        - exec: $%s
%s        - exec: ok
`, tag, forwardTag, ttlConfig)
}
