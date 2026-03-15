package main

func wiredServerForTest(s *server) *server {
	configureControlPlaneUseCases(s)
	return s
}
