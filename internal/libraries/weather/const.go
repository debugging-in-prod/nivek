package weather

// Forecast is nivek's view of a point forecast. Weather clients translate
// their vendor's response into this type; nothing outside the client file
// sees vendor wire types.
type Forecast struct {
	// TempF is the average surface temperature, rounded to the nearest
	// whole degree Fahrenheit.
	TempF int
}

// Location is nivek's view of where an IP address is. Geolocation clients
// translate their vendor's response into this type.
type Location struct {
	City     string
	Lat, Lon float64
}
