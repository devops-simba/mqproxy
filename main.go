package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/devops-simba/helpers"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Logging  *LoggingConfig `yaml:"logging,omitempty"`
	Metrics  *MetricsConfig `yaml:"metrics,omitempty"`
	Services map[string]MQTTServiceConfig
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to open config file: %w", err)
	}
	defer file.Close()

	configContent, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Failed to read config file's content: %w", err)
	}

	var config map[string]*Config
	err = yaml.Unmarshal(configContent, &config)
	if err != nil {
		return nil, fmt.Errorf("Invalid config file: %w", err)
	}

	if len(config) != 1 || config["proxy"] == nil {
		return nil, helpers.StringError("Config file format is not valid. Root of the config file must be `proxy`")
	}

	return config["proxy"], nil
}

func main() {
	var configFilePath string
	flag.StringVar(&configFilePath, "config", "./config.yml", "Path to the config file")
	flag.Parse()

	config, err := loadConfig(configFilePath)
	if err != nil {
		panic(err)
	}

	err = InitializeLogging(config.Logging)
	if err != nil {
		panic(err)
	}
	defer StopLogging()

	helpers.SetGlobalServiceExecuter(helpers.CreateServiceExecuter(GetLogFactory()))

	err = InitializeMetrics(config.Metrics)
	if err != nil {
		GetMainLogger().Fatalf("Failed to initialize metrics: %v", helpers.CContent(helpers.Orange, err))
	}
	defer StopMetrics()

	services := make([]helpers.Service, 0, len(config.Services))
	for svcName, svcConfig := range config.Services {
		service, enabled, err := CreateService(svcName, svcConfig)
		if err != nil {
			GetMainLogger().Fatalf("Failed to load service(%v): %v",
				helpers.CContent(helpers.Green, svcName),
				helpers.CContent(helpers.Orange, err))
		}
		if !enabled {
			GetMainLogger().Verbosef(1, "Ignoring service(%v) as it is not enabled",
				helpers.CContent(helpers.Green, svcName))
		}

		services = append(services, service)
	}

	stopRequested := make(chan struct{})
	svc := helpers.MergeServices("Application Services", services...)
	stopped := helpers.ExecuteServiceAsync(svc, stopRequested)

	helpers.WaitForApplicationTermination(func() {
		GetMainLogger().Debug("Close signal received")
		close(stopRequested)
	}, stopped)
}
