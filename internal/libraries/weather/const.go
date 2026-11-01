package weather

// Forecast is nivek's view of a point forecast. Weather clients translate
// their vendor's response into this type; nothing outside the client file
// sees vendor wire types.
type Forecast struct {
	// TempF is the average surface temperature, rounded to the nearest
	// whole degree Fahrenheit.
	TempF int
}
