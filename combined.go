package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	elasticsearch "github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type SystemInfo struct {
	CPUInfo     CPUInfo       `json:"CPUInfo"`
	ProcessInfo []ProcessInfo `json:"ProcessInfo"`
	DiskInfo    []DiskInfo    `json:"DiskInfo"`
}

type ProcessInfo struct {
	PID     int     `json:"PID"`
	User    string  `json:"User"`
	PR      int     `json:"PR"`
	NI      int     `json:"NI"`
	VIRT    int64   `json:"VIRT"`
	RES     int64   `json:"RES"`
	SHR     int64   `json:"SHR"`
	S       string  `json:"S"`
	CPU     float64 `json:"%CPU"`
	MEM     float64 `json:"%MEM"`
	Time    string  `json:"TIME+"`
	Command string  `json:"COMMAND"`
}

type CPUInfo struct {
	Architecture       string  `json:"Architecture"`
	CPUOpModes         string  `json:"CPUOpModes"`
	ByteOrder          string  `json:"ByteOrder"`
	CPUs               int     `json:"CPUs"`
	ThreadsPerCore     int     `json:"ThreadsPerCore"`
	CoresPerSocket     int     `json:"CoresPerSocket"`
	Sockets            int     `json:"Sockets"`
	NUMANodes          int     `json:"NUMANodes"`
	VendorID           string  `json:"VendorID"`
	CPUFamily          int     `json:"CPUFamily"`
	Model              int     `json:"Model"`
	ModelName          string  `json:"ModelName"`
	CPUMHz             float64 `json:"CPUMHz"`
	BogoMIPS           float64 `json:"BogoMIPS"`
	HypervisorVendor   string  `json:"HypervisorVendor"`
	VirtualizationType string  `json:"VirtualizationType"`
	L1DCache           string  `json:"L1DCache"`
	L1ICache           string  `json:"L1ICache"`
	L2Cache            string  `json:"L2Cache"`
	L3Cache            string  `json:"L3Cache"`
	NUMANode0CPUs      string  `json:"NUMANode0CPUs"`
	Flags              string  `json:"Flags"`
}

type DiskInfo struct {
	Filesystem string `json:"Filesystem"`
	Size       string `json:"Size"`
	Used       string `json:"Used"`
	Avail      string `json:"Avail"`
	UsePercent string `json:"UsePercent"`
	MountedOn  string `json:"MountedOn"`
}

func readAndParseCPUInfo(scanner *bufio.Scanner) (CPUInfo, error) {
	var cpuInfo CPUInfo

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		value := strings.Join(fields[1:], " ")

		switch key {
		case "Architecture:":
			cpuInfo.Architecture = value
		case "CPUOpModes":
			cpuInfo.CPUOpModes = value
		case "Byte":
			cpuInfo.ByteOrder = value
		case "CPU(s):":
			cpuInfo.CPUs, _ = strconv.Atoi(value)
		case "Thread(s)":
			cpuInfo.ThreadsPerCore, _ = strconv.Atoi(value)
		case "Core(s)":
			cpuInfo.CoresPerSocket, _ = strconv.Atoi(value)
		case "Socket(s):":
			cpuInfo.Sockets, _ = strconv.Atoi(value)
		case "NUMANode0CPUs ":
			cpuInfo.NUMANodes, _ = strconv.Atoi(value)
		case "VendorID ":
			cpuInfo.VendorID = value
		case "CPUFamily":
			cpuInfo.CPUFamily, _ = strconv.Atoi(value)
		case "Model:":
			cpuInfo.Model, _ = strconv.Atoi(value)
		case "ModelName":
			cpuInfo.ModelName = value
		case "CPUMHz":
			cpuInfo.CPUMHz, _ = strconv.ParseFloat(value, 64)
		case "BogoMIPS:":
			cpuInfo.BogoMIPS, _ = strconv.ParseFloat(value, 64)
		case "HypervisorVendor":
			cpuInfo.HypervisorVendor = value
		case "VirtualizationType ":
			cpuInfo.VirtualizationType = value
		case "L1DCache":
			cpuInfo.L1DCache = value
		case "L1ICache":
			cpuInfo.L1ICache = value
		case "L2Cache":
			cpuInfo.L2Cache = value
		case "L3Cache":
			cpuInfo.L3Cache = value
		case "NUMANode0CPUs":
			cpuInfo.NUMANode0CPUs = value
		case "Flags:":
			cpuInfo.Flags = value
		}
	}

	if err := scanner.Err(); err != nil {
		return CPUInfo{}, err
	}

	return cpuInfo, nil
}

func readAndParseTopOutput(scanner *bufio.Scanner) ([]ProcessInfo, error) {
	var processInfoList []ProcessInfo

	processInfoRegex := regexp.MustCompile(`^\s*(\d+)\s+(\S+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+([\d.]+)\s+([\d.]+)\s+(\S+)\s+(.*)$`)

	for scanner.Scan() {
		line := scanner.Text()

		if match := processInfoRegex.FindStringSubmatch(line); len(match) == 13 {
			pid, _ := strconv.Atoi(match[1])
			pr, _ := strconv.Atoi(match[3])
			ni, _ := strconv.Atoi(match[4])
			virt, _ := strconv.ParseInt(match[5], 10, 64)
			res, _ := strconv.ParseInt(match[6], 10, 64)
			shr, _ := strconv.ParseInt(match[7], 10, 64)
			cpu, _ := strconv.ParseFloat(match[9], 64)
			mem, _ := strconv.ParseFloat(match[10], 64)

			processInfo := ProcessInfo{
				PID:     pid,
				User:    match[2],
				PR:      pr,
				NI:      ni,
				VIRT:    virt,
				RES:     res,
				SHR:     shr,
				S:       match[8],
				CPU:     cpu,
				MEM:     mem,
				Time:    match[11],
				Command: match[12],
			}

			processInfoList = append(processInfoList, processInfo)
		}
	}

	return processInfoList, nil
}

func readAndParseDiskInfo(scanner *bufio.Scanner) ([]DiskInfo, error) {
	var diskInfolist []DiskInfo
	diskInfoRegex := regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s*$`)

	for scanner.Scan() {
		line := scanner.Text()
		if match := diskInfoRegex.FindStringSubmatch(line); len(match) == 7 {
			diskInfo := DiskInfo{
				Filesystem: match[1],
				Size:       match[2],
				Used:       match[3],
				Avail:      match[4],
				UsePercent: match[5],
				MountedOn:  match[6],
			}
			diskInfolist = append(diskInfolist, diskInfo)
		}
	}

	return diskInfolist, nil
}

func writeToJSONFile(filename string, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}

func runCommand(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command '%s' failed with error: %v", command, err)
	}
	return string(output), nil
}

func indexSystemInfoToElasticsearch(es *elasticsearch.Client, systemInfo SystemInfo) error {
	jsonData, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		return err
	}

	// Prepare the bulk data for Elasticsearch
	bulkRequest := esapi.IndexRequest{
		Index:      "unique",
		DocumentID: "",
		Body:       bytes.NewReader(jsonData),
		Refresh:    "true",
	}

	// Send the document to Elasticsearch
	res, err := bulkRequest.Do(context.Background(), es)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		var e map[string]interface{}
		err := json.NewDecoder(res.Body).Decode(&e)
		if err != nil {
			return err
		}
		return fmt.Errorf("error indexing document: %v", e)
	}

	fmt.Println("Indexed document:", string(jsonData))
	return nil
}

func main() {
	// Set up the Elasticsearch client
	cfg := elasticsearch.Config{
		Addresses: []string{"http://localhost:9200"},
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		fmt.Println("Error creating Elasticsearch client:", err)
		return
	}

	cpuCommand, err := runCommand("lscpu")
	if err != nil {
		fmt.Println("Error running 'lscpu' command:", err)
		return
	}
	cpuInfo, err := readAndParseCPUInfo(bufio.NewScanner(strings.NewReader(cpuCommand)))
	if err != nil {
		fmt.Println("Error parsing CPU info:", err)
		return
	}

	topCommand, err := runCommand("top -n 3 -b")
	if err != nil {
		fmt.Println("Error running 'top' command:", err)
		return
	}
	processInfoList, err := readAndParseTopOutput(bufio.NewScanner(strings.NewReader(topCommand)))
	if err != nil {
		fmt.Println("Error parsing top output:", err)
		return
	}

	sort.Slice(processInfoList, func(i, j int) bool {
		return processInfoList[i].CPU > processInfoList[j].CPU
	})

	if len(processInfoList) > 10 {
		processInfoList = processInfoList[:10]
	}

	diskCommand, err := runCommand("df -h")
	if err != nil {
		fmt.Println("Error running 'df' command:", err)
		return
	}

	diskInfoList, err := readAndParseDiskInfo(bufio.NewScanner(strings.NewReader(diskCommand)))
	if err != nil {
		fmt.Println("Error parsing disk info:", err)
		return
	}

	systemInfo := SystemInfo{
		CPUInfo:     cpuInfo,
		ProcessInfo: processInfoList,
		DiskInfo:    diskInfoList,
	}

	// Index the system information to Elasticsearch
	err = indexSystemInfoToElasticsearch(es, systemInfo)
	if err != nil {
		fmt.Println("Error indexing system info to Elasticsearch:", err)
		return
	}

	// Optionally, write the system information to a JSON file
	jsonData, err := json.MarshalIndent(systemInfo, "", "  ")
	if err != nil {
		fmt.Println("Error marshalling system info to JSON:", err)
		return
	}

	err = writeToJSONFile("system_info.json", string(jsonData))
	if err != nil {
		fmt.Println("Error writing system info to file:", err)
		return
	}

	fmt.Println("Output successfully written to system_info.json")
}
