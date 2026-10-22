package api

// WithOverpassEndpoint points the area builder at a different road data service.
// It exists for tests, which serve a fixture instead of calling the public
// Overpass API.
func WithOverpassEndpoint(endpoint string) Option {
	return func(s *Server) { s.overpassEndpoint = endpoint }
}
