/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import "flag"

// WebhookListenerOptions encapsulates the listener option and flags
type WebhookListenerOptions struct {
	TlsCert              string
	TlsKey               string
	Port                 int
	MetricsAddr          string
	ProbeAddr            string
	EnableLeaderElection bool
}

// our option instance
var Options = WebhookListenerOptions{
	TlsCert:              "",
	TlsKey:               "",
	Port:                 -999,
	MetricsAddr:          ":8080",
	ProbeAddr:            ":8081",
	EnableLeaderElection: false,
}

// use to load command line flags to options
func LoadEnterpriseValidatorOptions() {
	flag.StringVar(&Options.TlsCert, "tls-cert", "", "Certificate for TLS")
	flag.StringVar(&Options.TlsKey, "tls-key", "", "Private key file for TLS")
	flag.IntVar(&Options.Port, "port", 443, "Port to listen on for HTTPS traffic")
	flag.StringVar(&Options.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&Options.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&Options.EnableLeaderElection, "leader-elect", false, "Enable leader election for controller manager")
}
