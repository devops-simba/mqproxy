package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/devops-simba/helpers"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	mqproxyNamespace = "mqproxy"
	lbService        = "service"
	lbFrontend       = "frontend"
	lbBackend        = "backend"
	lbProtocol       = "protocol"
	lbNewBackend     = "new_backend_name"

	numConnectedClients   = "mqproxy_connected_clients"
	numRequests           = "mqproxy_proxy_requests_total"
	histogramResponseTime = "mqproxy_response_duration_seconds"
)

var (
	// Labels: service, frontend
	metricNumConnectedClients = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			//Namespace: mqproxyNamespace,
			Name: numConnectedClients,
			Help: "Number of active connections to this proxy",
		}, []string{lbService, lbFrontend, lbProtocol},
	)
	// Labels: service, frontend, backend
	metricNumRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			//Namespace: mqproxyNamespace,
			Name: numRequests,
			Help: "Number of requests to mqproxy",
		}, []string{lbService, lbFrontend, lbBackend},
	)
	// Labels: service, frontend, backend
	metricHistogramResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			//Namespace: mqproxyNamespace,
			Name: histogramResponseTime,
			Help: "Duration to answer a response",
		}, []string{lbService, lbFrontend, lbBackend},
	)

	metricsServer *http.Server   = nil
	metricsLogger helpers.Logger = nil
)

type MetricsConfig struct {
	Address     string                  `yaml:"address"`
	Enabled     *bool                   `yaml:"bool"`
	Certificate *CertificateInformation `yaml:"certificate"`
}

func InitializeMetrics(config *MetricsConfig) error {
	if config == nil {
		config = &MetricsConfig{Address: "http://:8080/metrics"}
	}

	if !GetOptionalBool(config.Enabled, true) {
		// metrics are disabled
		return nil
	}

	metricsLogger = CreateLogger("metrics")

	err := prometheus.Register(metricNumConnectedClients)
	if err != nil {
		metricsLogger.Errorf("Failed to register metric `%s`: %v", numConnectedClients, err)
		return err
	}

	err = prometheus.Register(metricNumRequests)
	if err != nil {
		metricsLogger.Errorf("Failed to register metric `%s`: %v", numRequests, err)
		return err
	}

	err = prometheus.Register(metricHistogramResponseTime)
	if err != nil {
		metricsLogger.Errorf("Failed to register metric `%s`: %v", histogramResponseTime, err)
		return err
	}

	if config.Address == "" {
		config.Address = "http://:8080/metrics/"
	}
	u, err := ParseUrl(config.Address, "http")
	if err != nil {
		metricsLogger.Errorf("%s is not a valid listen address: %v", config.Address, err)
		return err
	}

	mux := &http.ServeMux{}
	mux.Handle(GetUrlDirPath(u), promhttp.Handler())
	metricsServer = &http.Server{
		Addr:    net.JoinHostPort(GetUrlHostname(u), GetUrlPort(u)),
		Handler: mux,
	}

	metricsLogger.Debug("Listening for metric requests")
	switch u.Scheme {
	case "http":
		if config.Certificate != nil {
			return helpers.StringError("Certificate is not allowed for http protocol(metrics)")
		}
		go func() {
			err := metricsServer.ListenAndServe()
			if err != http.ErrServerClosed {
				metricsLogger.Errorf("Metrics server stopped: %v", err)
			}
		}()

	case "https":
		if config.Certificate == nil {
			return helpers.StringError("Certificate is required for https protocol(metrics)")
		}

		go func() {
			err := metricsServer.ListenAndServeTLS(config.Certificate.CertificateFile, config.Certificate.PrivateKeyFile)
			if err != http.ErrServerClosed {
				metricsLogger.Errorf("Metrics server stopped: %v", err)
			}
		}()

	default:
		return helpers.StringError("Invalid metrics protocol")
	}

	return nil
}
func StopMetrics() {
	if metricsServer != nil {
		metricsLogger.Verbose(10, "Stopping metrics server")
		metricsServer.Shutdown(context.Background())
	}
}

func OnClientConnect(serviceName, frontend, protocol string) {
	if metricsServer == nil {
		return
	}

	//metricsLogger.Debugf("ClientConnect{service: `%s`, frontend_proto: `%s`, frontend_name: `%s`}", serviceName, frontend, protocol)
	g := metricNumConnectedClients.WithLabelValues(serviceName, frontend, protocol)
	g.Inc()
}
func OnClientDisconnect(serviceName, frontend, protocol string) {
	if metricsServer == nil {
		return
	}

	//metricsLogger.Debugf("ClientDisconnect{service: `%s`, frontend_proto: `%s`, frontend_name: `%s`}", serviceName, frontend, protocol)
	g := metricNumConnectedClients.WithLabelValues(serviceName, frontend, protocol)
	g.Dec()
}

func OnRequestReceived(serviceName, frontend, backend string) {
	if metricsServer == nil {
		return
	}

	//metricsLogger.Debugf("RequestReceived{service: `%s`, frontend: `%s`, backend: `%s`}", serviceName, frontend, backend)
	c := metricNumRequests.WithLabelValues(serviceName, frontend, backend)
	c.Inc()
}

func OnResponse(serviceName, frontend, backend string, duration time.Duration) {
	if metricsServer == nil {
		return
	}

	//metricsLogger.Debugf("Response{service: `%s`, frontend: `%s`, backend: `%s`, duration: `%v`}", serviceName, frontend, backend, duration)
	o := metricHistogramResponseTime.WithLabelValues(serviceName, frontend, backend)
	o.Observe(duration.Seconds())
}
