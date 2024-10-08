package coremain

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func Eatlist(args []string) {
	if len(args) < 1 {
		os.Exit(1)
	}
	switch args[0] {
	case "list":
		if len(args) < 3 {
			os.Exit(1)
		}
		outputPath := args[1]
		inputFiles := args[2:]
		processForcelist(inputFiles, outputPath)
	case "cut":
		cut()
	case "spilt":
		spiltData()
	case "trackerslist":
		processTrackersList()
	case "ttl_rules":
		processForceTTLRules()
	case "hosts":
		processHosts()
	case "calc":
		if len(args) < 4 {
			os.Exit(1)
		}
		ulimit := args[1]
		numThreads := args[2]
		mode := args[3]
		calc(ulimit, numThreads, mode)
	default:
		fmt.Println("Unknown command")
		os.Exit(1)
	}
}
func cut() {
	inputFilePath := "/data/global_mark.dat"
	outputDir := "/tmp/global_mark"

	fileName := filepath.Base(inputFilePath)
	fileExt := filepath.Ext(inputFilePath)
	fileBase := fileName[:len(fileName)-len(fileExt)]

	outputFilePath1 := filepath.Join(outputDir, fileBase+".dat.xz")
	outputFilePath2 := filepath.Join(outputDir, fileBase+".dat.sha")

	inputFile, err := os.Open(inputFilePath)
	if err != nil {
		fmt.Println("failed to open input file:", err)
		return
	}
	defer inputFile.Close()

	fileInfo, err := inputFile.Stat()
	if err != nil {
		fmt.Println("failed to get file info:", err)
		return
	}

	fileSize := fileInfo.Size()
	if fileSize <= 1024 {
		fmt.Println("file is too small, less than or equal to 1024 bytes")
		return
	}

	cutPoint := fileSize - 1024

	outputFile1, err := os.Create(outputFilePath1)
	if err != nil {
		fmt.Println("failed to create output file 1:", err)
		return
	}
	defer outputFile1.Close()

	outputFile2, err := os.Create(outputFilePath2)
	if err != nil {
		fmt.Println("failed to create output file 2:", err)
		return
	}
	defer outputFile2.Close()

	_, err = inputFile.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Println("failed to seek input file:", err)
		return
	}
	_, err = io.CopyN(outputFile1, inputFile, cutPoint)
	if err != nil {
		fmt.Println("failed to copy first part of file:", err)
		return
	}

	_, err = inputFile.Seek(cutPoint, io.SeekStart)
	if err != nil {
		fmt.Println("failed to seek input file to cut point:", err)
		return
	}
	_, err = io.Copy(outputFile2, inputFile)
	if err != nil {
		fmt.Println("failed to copy last 1024 bytes of file:", err)
		return
	}
}

func spiltData() {
	inputFile := "/tmp/global_mark/global_mark.dat"
	outputFiles := []string{
		"/tmp/global_mark.dat",
		"/tmp/global_mark_cn.dat",
		"/tmp/cn_mark.dat",
	}
	patterns := []string{
		`^domain:[-_.A-Za-z0-9]+$`,
		`^##@@domain:[-_.A-Za-z0-9]+$`,
		`^#@domain:[-_.A-Za-z0-9]+$`,
	}

	var wg sync.WaitGroup
	results := make([]map[string]struct{}, 3)

	for i := range patterns {
		results[i] = make(map[string]struct{})
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			processFile(inputFile, patterns[index], results[index], index)
		}(i)
	}

	wg.Wait()

	for i, result := range results {
		writeOutput(outputFiles[i], result)
	}
}

func processFile(inputFile, pattern string, result map[string]struct{}, index int) {
	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	re := regexp.MustCompile(pattern)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if re.MatchString(line) {
			line = processLine(line, index)
			if strings.ContainsAny(line, "abcdefghijklmnopqrstuvwxyz") && len(line) > 0 {
				result[line] = struct{}{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
	}
}

func processLine(line string, index int) string {
	switch index {
	case 1:
		return strings.Replace(line, "##@@domain:", "domain:", 1)
	case 2:
		return strings.Replace(line, "#@domain:", "domain:", 1)
	default:
		return line
	}
}

func writeOutput(outputFile string, result map[string]struct{}) {
	file, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	lines := make([]string, 0, len(result))
	for line := range result {
		lines = append(lines, line)
	}
	sort.Strings(lines)

	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
}
func processTrackersList() {
	file, err := os.Open("/data/trackerslist.txt")
	if err != nil {
		fmt.Printf("Error opening tracker list file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	result := make(map[string]struct{})
	scanner := bufio.NewScanner(file)

	urlRegex := regexp.MustCompile(`^[a-z]+://.+`)
	domainRegex := regexp.MustCompile(`\.[a-z]`)
	validCharsRegex := regexp.MustCompile(`[-._0-9a-zA-Z]+`)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if urlRegex.MatchString(line) {
			parts := strings.SplitN(line, "//", 2)
			if len(parts) > 1 {
				domain := parts[1]
				domain = strings.SplitN(domain, "/", 2)[0]
				domain = strings.SplitN(domain, ":", 2)[0]
				if domainRegex.MatchString(domain) && validCharsRegex.MatchString(domain) {
					result["full:"+domain] = struct{}{}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading tracker list file: %v\n", err)
		os.Exit(1)
	}
	writeOutput("/tmp/cn_tracker_list.txt", result)
}
func processForceTTLRules() {
	file, err := os.Open("/data/force_ttl_rules.txt")
	if err != nil {
		fmt.Printf("Error opening force TTL rules file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	txtRules := make(map[string]struct{})
	tomlRules := make(map[string]struct{})
	cloakingRules := make(map[string]struct{})

	domainRegex := regexp.MustCompile(`^[-._A-Za-z0-9]+$`)
	validSpecialCharRegex := regexp.MustCompile(`^[-._A-Za-z0-9*\[\]]+$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Split(line, "@")
		switch len(parts) {
		case 2: // Forwarding rule
			ruleDomain := parts[0]
			ruleADNS := parts[1]
			fmt.Println(ruleDomain, ruleADNS)
			if domainRegex.MatchString(ruleDomain) {
				txtRules["domain:"+ruleDomain] = struct{}{}
				tomlRules[fmt.Sprintf("%s %s", ruleDomain, ruleADNS)] = struct{}{}
			}
		case 3: // Cloaking rule
			ruleDomain := parts[0]
			ruleCloaking := parts[2]
			fmt.Println("Cloaking rule", ruleDomain, ruleCloaking)
			if domainRegex.MatchString(ruleDomain) {
				fmt.Println("Valid1 domain:", ruleDomain)
				txtRules["domain:"+ruleDomain] = struct{}{}
				cloakingRules[fmt.Sprintf("%s %s", ruleDomain, ruleCloaking)] = struct{}{}
			} else if validSpecialCharRegex.MatchString(ruleDomain) {
				fmt.Println("Special char domain:", ruleDomain)
				regexpRuleDomain := strings.ReplaceAll(ruleDomain, ".", "\\.")
				regexpRuleDomain = strings.ReplaceAll(regexpRuleDomain, "*", ".*")
				regexpRuleDomain += "$"
				txtRules["regexp:"+regexpRuleDomain] = struct{}{}
				cloakingRules[fmt.Sprintf("%s %s", ruleDomain, ruleCloaking)] = struct{}{}
			} else {
				fmt.Println("Invalid domain:", ruleDomain)
			}
		case 4: // Full Cloaking rule
			ruleDomain := parts[0]
			ruleCloaking := parts[3]
			fmt.Println(ruleDomain, ruleCloaking)
			if domainRegex.MatchString(ruleDomain) {
				txtRules["full:"+ruleDomain] = struct{}{}
				cloakingRules[fmt.Sprintf("=%s %s", ruleDomain, ruleCloaking)] = struct{}{}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading force TTL rules file: %v\n", err)
		return
	}

	writeOutput("/tmp/force_ttl_rules.txt", txtRules)
	writeOutput("/tmp/force_ttl_rules.toml", tomlRules)
	writeOutput("/tmp/force_ttl_rules_cloaking.toml", cloakingRules)
}
func processHosts() {
	file, err := os.Open("/etc/hosts")
	if err != nil {
		fmt.Printf("Error opening /etc/hosts file: %v\n", err)
		return
	}
	defer file.Close()

	linesMap := make(map[string]struct{})

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			linesMap[line] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading /etc/hosts file: %v\n", err)
		return
	}

	result := make(map[string]struct{})
	for line := range linesMap {
		var record string
		var domain string

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			if ip := net.ParseIP(parts[0]); ip != nil {
				record = parts[0]
				domain = parts[1]
			} else if ip := net.ParseIP(parts[1]); ip != nil {
				record = parts[1]
				domain = parts[0]
			}
		}

		if domain != "" && record != "" {
			result[fmt.Sprintf("%s %s", domain, record)] = struct{}{}
		}
	}

	writeOutput("/tmp/hosts.txt", result)
}

func calc(ulimitStr string, numThreadsStr string, mode string) {
	ulimit, err1 := strconv.Atoi(ulimitStr)
	if err1 != nil {
		ulimit = 10240
	}

	numThreads, err2 := strconv.Atoi(numThreadsStr)
	if err2 != nil {
		numThreads = 2
	}

	var adjustedUlimit float64

	switch mode {
	case "r":
		adjustedUlimit = float64(ulimit) * 0.7
	case "f":
		adjustedUlimit = float64(ulimit) * 0.2
	default:
		adjustedUlimit = float64(ulimit)
	}

	outgoingRange := int(math.Max(200, math.Min(adjustedUlimit-100, adjustedUlimit/float64(2*numThreads))))
	if outgoingRange > 8192 {
		outgoingRange = 8192
	}

	numQueriesPerThread := int(math.Max(100, float64(outgoingRange/(2*numThreads))))
	if numQueriesPerThread > 4096 {
		numQueriesPerThread = 4096
	}

	fmt.Printf("outgoing:%d:outgoing_half:%d:num-queries-per-thread:%d",
		outgoingRange, outgoingRange/2, numQueriesPerThread)
}

func processForcelist(caseList []string, outputPath string) {
	result := make(map[string]struct{})

	for _, filePath := range caseList {
		file, err := os.Open(filePath)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			if !regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(line) {
				continue
			}

			if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				prefix := parts[0]
				if prefix != "domain" && prefix != "full" && prefix != "regexp" && prefix != "keyword" {
					continue
				}
			} else {
				line = "domain:" + line
			}

			result[line] = struct{}{}
		}
	}
	writeOutput(outputPath, result)
}
