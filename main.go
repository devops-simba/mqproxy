package main

import (
	"flag"
	"io/ioutil"
	"os"
	"sync"

	log "github.com/golang/glog"
)

func Readenv(key string, defaultValue string) string {
	value, ok := os.LookupEnv(key)
	if ok {
		return value
	} else {
		return defaultValue
	}
}
func Initialize() []*MQTTFrontend {
	configPath := flag.String("config", Readenv("CONFIG_PATH", "/etc/mqproxy/conf.yml"), "Path to the config file")
	flag.Parse()

	configData, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config from the file: %v", err)
	}

	servers, err := LoadConfig(configData)
	if err != nil {
		log.Fatalf("Invalid config format: %v", err)
	}

	return servers
}

func main() {
	servers := Initialize()
	stop := make(chan struct{})
	stopped := make([]<-chan struct{}, 0)
	//
	for i := 0; i < len(servers); i++ {
		ch, err := servers[i].Run(stop)
		if err != nil {
			// stop all previous servers
			close(stop)
			for _, c := range stopped {
				<-c
			}
			log.Fatalf("Failed to load server: %v", err)
		}

		stopped = append(stopped, ch)
	}

	// now that all servers are running, we must wait for end signal
	allDone := sync.WaitGroup{}
	for i := 0; i < len(servers); i++ {
		allDone.Add(1)
		go func(index int) {
			<-stopped[index]
			allDone.Done()
			log.Infof("frontend `%s` stopped", servers[index].Connector.GetAddress())
		}(i)
	}

	allFrontendsStopped := make(chan struct{})
	go func() {
		allDone.Wait()
		close(allFrontendsStopped)
	}()

	stopSignal := SetupSignalHandler(1)
	select {
	case <-allFrontendsStopped:
	case <-stopSignal:
		close(stop)
		<-allFrontendsStopped
	}
}
