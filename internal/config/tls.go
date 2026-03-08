package config

type TLSConfig struct {
	Enable            bool   `json:"enable"`
	CertFile          string `json:"cert_file"`
	KeyFile           string `json:"key_file"`
	ClientCAFile      string `json:"client_ca_file"`
	RequireClientCert bool   `json:"require_client_cert"`
}

func defaultTLSConfig() TLSConfig {
	return TLSConfig{Enable: false}
}
