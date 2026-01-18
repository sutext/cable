package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Create a new mux
	mux := http.NewServeMux()

	// Add a simple health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Serve swagger.json directly from the parent docs directory
	mux.HandleFunc("/swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../docs/swagger.json")
	})

	// Add a simple Swagger UI HTML page that loads swagger.json directly
	mux.HandleFunc("/swagger", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html>
		<head>
			<title>Swagger UI</title>
			<link rel="stylesheet" type="text/css" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
		</head>
		<body>
			<div id="swagger-ui"></div>
			<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
			<script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-standalone-preset.js"></script>
			<script>
				const ui = SwaggerUIBundle({
					url: "/swagger/swagger.json",
					dom_id: '#swagger-ui',
					deepLinking: true,
					docExpansion: 'list',
					presets: [
						SwaggerUIBundle.presets.apis,
						SwaggerUIStandalonePreset
					],
					layout: "StandaloneLayout"
				});
			</script>
		</body>
		</html>`)
	})

	// Redirect /swagger/index.html to /swagger
	mux.HandleFunc("/swagger/index.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger", http.StatusMovedPermanently)
	})

	// Start the server
	log.Printf("Starting test server on http://localhost:8080")
	log.Printf("Swagger UI available at http://localhost:8080/swagger")
	log.Printf("or http://localhost:8080/swagger/index.html")
	log.Fatal(http.ListenAndServe(":8080", mux))
}