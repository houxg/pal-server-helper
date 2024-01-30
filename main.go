package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"pal-server-helper/pal"

	"github.com/shirou/gopsutil/mem"
)

type HelperConfig struct {
	IP               string  `json:"ip"`
	Port             int     `json:"port"`
	Password         string  `json:"password"`
	RetryCount       int     `json:"retryCount"`
	RetryDelay       int     `json:"retryDelay"`
	RebootScriptPath string  `json:"rebootScriptPath"`
	RebootSeconds    int     `json:"rebootSeconds"`
	OOMThreshold     float64 `json:"oomThreshold"`
	OOMCheckInterval int     `json:"oomCheckIntervalSeconds"`
}

func main() {
	// 定义命令行参数
	configFilePath := flag.String("c", "", "Path to the configuration file")
	flag.Parse()
	if *configFilePath == "" {
		log("please use -config to setup config file path")
		return
	}

	log("pal server helper started")
	log("loading config...")
	config, err := loadConfig(*configFilePath)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}
	log("Connect to server...")
	client := pal.NewPalClient(config.IP, config.Port, config.Password, config.RetryDelay, config.RetryCount)
	connectErr := client.Connect()
	if connectErr != nil {
		fmt.Println(connectErr)
		return
	}
	defer client.Close()

	go monitorMemoryUsage(client, config)

	// 主程序持续执行
	for {
		time.Sleep(10 * time.Second) // 可以根据实际需求调整间隔时间
		// 在这里可以添加其他持续执行的逻辑
	}
}

func log(message string) {
	currentTime := time.Now()
	formattedTime := currentTime.Format("2006-01-02 15:04:05")
	fmt.Println(formattedTime + ": " + message)
}

func loadConfig(filename string) (HelperConfig, error) {
	file, err := os.Open(filename)
	if err != nil {
		return HelperConfig{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	config := HelperConfig{}
	err = decoder.Decode(&config)
	if err != nil {
		return HelperConfig{}, err
	}

	return config, nil
}

func monitorMemoryUsage(client *pal.PalClient, config HelperConfig) {
	ticker := time.NewTicker(time.Duration(config.OOMCheckInterval) * time.Second)
	defer ticker.Stop()

	var wg sync.WaitGroup

	for {
		select {
		case <-ticker.C:
			memory, err := mem.VirtualMemory()
			if err != nil {
				fmt.Println("Failed to get memory info:", err)
				continue
			}

			usedPercent := memory.UsedPercent
			log(fmt.Sprintf("Current memory usage: %.2f%%\n", usedPercent))
			if usedPercent > config.OOMThreshold {
				wg.Add(2)
				notifyReboot(&wg, client, config)
				reboot(&wg, client, config)
				wg.Wait() // 阻塞，直到重启完成
			}
		}
	}
}

func notifyReboot(wg *sync.WaitGroup, client *pal.PalClient, config HelperConfig) {
	defer wg.Done()
	// 执行通知重启操作
	log("Notify to reboot")
	seconds := strconv.Itoa(config.RebootSeconds)
	client.Reboot(seconds)
	countdown := config.RebootSeconds
	for countdown > 0 {
		message := fmt.Sprintf("OOM_server_reboot_in_%ds", countdown)
		client.Broadcast(message)
		time.Sleep(time.Second)
		countdown--
	}
}

func reboot(wg *sync.WaitGroup, client *pal.PalClient, config HelperConfig) {
	defer wg.Done()
	log("closing RCON client")
	client.Close()
	log("Waiting to restart...")
	time.Sleep(10 * time.Second)
	// 执行重启操作
	log("Rebooting...")
	cmd := exec.Command("sh", config.RebootScriptPath)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Failed to execute reboot script:", err)
	} else {
		log("Reboot script executed successfully")
		client.Close()
		// 阻塞2分钟，等待服务重启
		log("Waiting for service to restart...")
		time.Sleep(2 * time.Minute)
		log("Service restarted")
	}
}
