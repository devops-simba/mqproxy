module github.com/devops-simba/mqproxy

go 1.15

require (
	github.com/devops-simba/helpers v1.0.12
	github.com/eclipse/paho.mqtt.golang v1.2.0
	github.com/gorilla/websocket v1.4.2
	github.com/prometheus/client_golang v1.8.0
	google.golang.org/appengine v1.4.0
	gopkg.in/yaml.v2 v2.3.0
)

replace github.com/devops-simba/helpers => /usr/local/src/go.1.15/src/github.com/devops-simba/helpers
