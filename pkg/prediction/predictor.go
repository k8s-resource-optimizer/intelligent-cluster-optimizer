package prediction

import (
	"fmt"
	"time"
)

// Forecast represents a predicted future value
type Forecast struct {
	// Timestamp of the forecasted value
	Timestamp time.Time

	// Value is the predicted value
	Value float64

	// Lower bound of confidence interval
	LowerBound float64

	// Upper bound of confidence interval
	UpperBound float64

	// Confidence level (e.g., 0.95 for 95% confidence interval)
	ConfidenceLevel float64

	// HorizonIndex is how many periods ahead this forecast is (1 = next period)
	HorizonIndex int
}

// ForecastResult contains the complete forecasting output
type ForecastResult struct {
	// Forecasts contains predicted future values
	Forecasts []Forecast

	// FittedValues contains the model's fitted values for historical data
	FittedValues []float64

	// Residuals contains the errors (actual - fitted)
	Residuals []float64

	// Model parameters
	Parameters *ModelParameters

	// Error metrics
	Metrics *ErrorMetrics

	// Seasonal components (if applicable)
	SeasonalComponents []float64

	// Trend components (if applicable)
	TrendComponents []float64

	// Method used for forecasting
	Method string
}

// ModelParameters contains the fitted model parameters
type ModelParameters struct {
	// Alpha - level smoothing parameter (0 < alpha < 1)
	Alpha float64

	// Beta - trend smoothing parameter (0 < beta < 1)
	Beta float64

	// Gamma - seasonal smoothing parameter (0 < gamma < 1)
	Gamma float64

	// SeasonalPeriod - number of observations per season
	SeasonalPeriod int

	// InitialLevel - starting level value
	InitialLevel float64

	// InitialTrend - starting trend value
	InitialTrend float64

	// InitialSeasonals - starting seasonal values
	InitialSeasonals []float64

	// SeasonalType - "additive" or "multiplicative"
	SeasonalType SeasonalType

	// TrendType - "additive", "multiplicative", or "none"
	TrendType TrendType
}

// ErrorMetrics contains forecast accuracy metrics
type ErrorMetrics struct {
	// MAE - Mean Absolute Error
	MAE float64

	// MSE - Mean Squared Error
	MSE float64

	// RMSE - Root Mean Squared Error
	RMSE float64

	// MAPE - Mean Absolute Percentage Error
	MAPE float64

	// SMAPE - Symmetric Mean Absolute Percentage Error
	SMAPE float64
}

// SeasonalType defines how seasonality is modeled
type SeasonalType string

const (
	SeasonalAdditive       SeasonalType = "additive"
	SeasonalMultiplicative SeasonalType = "multiplicative"
	SeasonalNone           SeasonalType = "none"
)

// TrendType defines how trend is modeled
type TrendType string

const (
	TrendAdditive       TrendType = "additive"
	TrendMultiplicative TrendType = "multiplicative"
	TrendNone           TrendType = "none"
	TrendDamped         TrendType = "damped"
)

// Predictor is the interface for time series forecasting methods
type Predictor interface {
	// Fit trains the model on historical data
	Fit(data []float64) error

	// FitWithTimestamps trains the model with timestamp information
	FitWithTimestamps(data []float64, timestamps []time.Time) error

	// Predict generates forecasts for n periods ahead
	Predict(horizons int) (*ForecastResult, error)

	// FitPredict combines fitting and predicting in one step
	FitPredict(data []float64, horizons int) (*ForecastResult, error)

	// Name returns the predictor's method name
	Name() string
}

// Config contains configuration for prediction
type Config struct {
	// SeasonalPeriod - number of observations per season
	// e.g., 24 for hourly data with daily seasonality
	// e.g., 7 for daily data with weekly seasonality
	SeasonalPeriod int

	// Alpha - level smoothing (0 = auto-optimize)
	Alpha float64

	// Beta - trend smoothing (0 = auto-optimize)
	Beta float64

	// Gamma - seasonal smoothing (0 = auto-optimize)
	Gamma float64

	// SeasonalType - how to model seasonality
	SeasonalType SeasonalType

	// TrendType - how to model trend
	TrendType TrendType

	// DampingFactor for damped trend (0.8-0.98 typical)
	DampingFactor float64

	// ConfidenceLevel for prediction intervals (default: 0.95)
	ConfidenceLevel float64

	// MinDataPoints - minimum data points required for fitting
	MinDataPoints int
}

// DefaultConfig returns default prediction configuration (hourly data, daily seasonality).
func DefaultConfig() *Config {
	return &Config{
		SeasonalPeriod:  24, // Hourly data with daily seasonality
		Alpha:           0,  // Auto-optimize
		Beta:            0,  // Auto-optimize
		Gamma:           0,  // Auto-optimize
		SeasonalType:    SeasonalAdditive,
		TrendType:       TrendAdditive,
		DampingFactor:   0.9,
		ConfidenceLevel: 0.95,
		MinDataPoints:   2, // At least 2 seasons
	}
}

// PodMetricsConfig returns Holt-Winters config tuned for 30-second pod CPU samples.
// SeasonalPeriod=120 covers a 1-hour cycle; TrendDamped prevents long-horizon drift.
func PodMetricsConfig() *Config {
	return &Config{
		SeasonalPeriod:  120,
		Alpha:           0,
		Beta:            0,
		Gamma:           0,
		SeasonalType:    SeasonalAdditive,
		TrendType:       TrendDamped,
		DampingFactor:   0.85,
		ConfidenceLevel: 0.95,
		MinDataPoints:   1,
	}
}

// Summary returns a human-readable summary of the forecast result
func (r *ForecastResult) Summary() string {
	if len(r.Forecasts) == 0 {
		return "No forecasts generated"
	}

	lastForecast := r.Forecasts[len(r.Forecasts)-1]
	return fmt.Sprintf("Forecast: %d periods ahead, last value=%.2f [%.2f, %.2f], RMSE=%.4f",
		len(r.Forecasts), lastForecast.Value, lastForecast.LowerBound, lastForecast.UpperBound,
		r.Metrics.RMSE)
}

// GetForecast returns the forecast for a specific horizon (1-indexed)
func (r *ForecastResult) GetForecast(horizon int) (*Forecast, error) {
	if horizon < 1 || horizon > len(r.Forecasts) {
		return nil, fmt.Errorf("horizon %d out of range [1, %d]", horizon, len(r.Forecasts))
	}
	return &r.Forecasts[horizon-1], nil
}

// PeakForecast returns the maximum forecasted value
func (r *ForecastResult) PeakForecast() *Forecast {
	if len(r.Forecasts) == 0 {
		return nil
	}

	peak := &r.Forecasts[0]
	for i := range r.Forecasts {
		if r.Forecasts[i].Value > peak.Value {
			peak = &r.Forecasts[i]
		}
	}
	return peak
}

// TroughForecast returns the minimum forecasted value
func (r *ForecastResult) TroughForecast() *Forecast {
	if len(r.Forecasts) == 0 {
		return nil
	}

	trough := &r.Forecasts[0]
	for i := range r.Forecasts {
		if r.Forecasts[i].Value < trough.Value {
			trough = &r.Forecasts[i]
		}
	}
	return trough
}
