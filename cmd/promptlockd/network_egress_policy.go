package main

func (s *server) validateNetworkEgress(cmd []string, intent string) error {
	return s.controlPolicy().ValidateNetworkEgress(cmd, intent)
}
