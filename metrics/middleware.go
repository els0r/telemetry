package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var defaultMetricPath = "/metrics"

// Prometheus contains the metrics gathered by the instance and its path. It's been stripped and adapted from
// https://github.com/zsais/go-gin-prometheus/blob/master/middleware.go, the main reason being the lack of support
// for histogram buckets.
type Prometheus struct {
	reqCnt *prometheus.CounterVec
	reqSz  *prometheus.HistogramVec
	resSz  *prometheus.HistogramVec

	// request duration can be configured
	reqDur         *prometheus.HistogramVec
	reqDurHistOpts *prometheus.HistogramOpts
	reqDurLabels   []string

	additionalMetrics []prometheus.Collector

	registry *prometheus.Registry

	metricsPath string

	// gin.Context key to use as the "path" label value.
	// If set, the value from c.Get(pathLabelFromContext) is used instead of c.Request.URL.Path.
	pathLabelFromContext string
}

// WithMetricsPath sets the metrics path to `path`. The default is `/metrics`.
func (p *Prometheus) WithMetricsPath(path string) *Prometheus {
	p.metricsPath = path
	return p
}

// WithNativeHistograms enables the use of the native prometheus histogram. This is still an experimental feature.
func (p *Prometheus) WithNativeHistograms(enabled bool) *Prometheus { //revive:disable-line
	if enabled {
		// Experimental: see documentation on NewHistogram for buckets explanation
		p.reqDurHistOpts.NativeHistogramBucketFactor = 1.1
		p.reqDur = prometheus.NewHistogramVec(*p.reqDurHistOpts, p.reqDurLabels)
	}
	return p
}

// WithRequestDurationBuckets overrides the default buckets for the request duration histogram.
func (p *Prometheus) WithRequestDurationBuckets(buckets []float64) *Prometheus {
	p.reqDurHistOpts.Buckets = buckets
	p.reqDur = prometheus.NewHistogramVec(*p.reqDurHistOpts, p.reqDurLabels)
	return p
}

// WithPathLabelFromContext sets a gin.Context key whose value will be used as the "path"
// label instead of c.Request.URL.Path. This is useful for grouping routes with path parameters.
func (p *Prometheus) WithPathLabelFromContext(key string) *Prometheus {
	p.pathLabelFromContext = key
	return p
}

// NewPrometheus generates a new set of metrics with a certain subsystem name. If additionalMetrics is supplied,
// it will register those as well. The `With...` modifiers are meant to be called _before_ they are used/registered
// with gin. Best idea is to call them immediately after NewPrometheus().
//
// If registry is nil, a new prometheus.Registry is created. Pass prometheus.DefaultRegisterer's
// underlying *prometheus.Registry to use the global registry (not recommended for testing).
func NewPrometheus(serviceName, subsystem string, registry *prometheus.Registry, additionalMetrics ...prometheus.Collector) *Prometheus {
	if registry == nil {
		registry = prometheus.NewRegistry()
		// register standard Go and process collectors on custom registries
		registry.MustRegister(prometheus.NewGoCollector())
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	}

	p := &Prometheus{
		metricsPath: defaultMetricPath,
		registry:    registry,
		// request duration metric configuration
		reqDurHistOpts: &prometheus.HistogramOpts{
			Namespace: serviceName,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "HTTP request latencies in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		reqDurLabels: []string{"code", "method", "path"},
	}
	p.newMetrics(serviceName, subsystem, additionalMetrics...)
	return p
}

// SetMetricsPath sets the metrics path in the gin.Engine. To control the value of the path, use (*Prometheus).WithMetricsPath.
func (p *Prometheus) SetMetricsPath(e *gin.Engine) {
	e.GET(p.metricsPath, p.prometheusHandler())
}

// SetMetricsPathWithAuth set metrics paths with authentication in the gin.Engine.
func (p *Prometheus) SetMetricsPathWithAuth(e *gin.Engine, accounts gin.Accounts) {
	e.GET(p.metricsPath, gin.BasicAuth(accounts), p.prometheusHandler())
}

func (p *Prometheus) newMetrics(serviceName, subsystem string, additionalMetrics ...prometheus.Collector) {
	// default metrics provided by the middleware library
	reqCntLabels := []string{"host", "handler"}
	reqCntLabels = append(reqCntLabels, p.reqDurLabels...)

	p.reqCnt = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: serviceName,
			Subsystem: subsystem,
			Name:      "requests_total",
			Help:      "How many HTTP requests processed, partitioned by status code and HTTP method",
		},
		reqCntLabels,
	)
	p.reqDur = prometheus.NewHistogramVec(
		*p.reqDurHistOpts,
		p.reqDurLabels,
	)
	p.reqSz = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: serviceName,
			Subsystem: subsystem,
			Name:      "request_size_bytes",
			Help:      "HTTP request sizes in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"code", "method"},
	)
	p.resSz = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: serviceName,
			Subsystem: subsystem,
			Name:      "response_size_bytes",
			Help:      "HTTP response sizes in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"code", "method"},
	)
	p.additionalMetrics = additionalMetrics
}

// RegisterMetrics registers all metric collectors with the prometheus registry.
// Call this before serving MetricsHandler() in non-gin (net/http) setups.
func (p *Prometheus) RegisterMetrics() {
	var toRegister []prometheus.Collector
	toRegister = append(toRegister,
		p.reqCnt,
		p.reqDur,
		p.reqSz, p.resSz,
	)
	toRegister = append(toRegister, p.additionalMetrics...)

	for _, collector := range toRegister {
		p.registry.MustRegister(collector)
	}
}

func (p *Prometheus) use(e *gin.Engine) {
	p.RegisterMetrics()
	e.Use(p.GinHandlerFunc())
}

// Register adds the middleware to a gin engine.
func (p *Prometheus) Register(e *gin.Engine) {
	p.use(e)
	p.SetMetricsPath(e)
}

// RegisterWithAuth adds the middleware to a gin engine with BasicAuth.
func (p *Prometheus) RegisterWithAuth(e *gin.Engine, accounts gin.Accounts) {
	p.use(e)
	p.SetMetricsPathWithAuth(e, accounts)
}

// HandlerFunc returns a net/http middleware that records metrics.
// This is the framework-agnostic version of GinHandlerFunc.
func (p *Prometheus) HandlerFunc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == p.metricsPath {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		reqSz := computeApproximateRequestSize(r)

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		status := strconv.Itoa(rw.status)
		elapsed := float64(time.Since(start)) / float64(time.Second)
		resSz := float64(rw.size)

		path := r.URL.Path
		p.reqDur.WithLabelValues(status, r.Method, path).Observe(elapsed)
		p.reqCnt.WithLabelValues(r.Host, r.URL.Path, status, r.Method, path).Inc()
		p.reqSz.WithLabelValues(status, r.Method).Observe(float64(reqSz))
		p.resSz.WithLabelValues(status, r.Method).Observe(resSz)
	})
}

// GinHandlerFunc defines handler function for gin middleware.
func (p *Prometheus) GinHandlerFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == p.metricsPath {
			c.Next()
			return
		}

		start := time.Now()
		reqSz := computeApproximateRequestSize(c.Request)

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		elapsed := float64(time.Since(start)) / float64(time.Second)
		resSz := float64(c.Writer.Size())

		path := c.Request.URL.Path
		if len(p.pathLabelFromContext) > 0 {
			pp, found := c.Get(p.pathLabelFromContext)
			if found {
				if s, ok := pp.(string); ok {
					path = s
				}
			}
		}
		p.reqDur.WithLabelValues(status, c.Request.Method, path).Observe(elapsed)
		p.reqCnt.WithLabelValues(c.Request.Host, c.HandlerName(), status, c.Request.Method, path).Inc()
		p.reqSz.WithLabelValues(status, c.Request.Method).Observe(float64(reqSz))
		p.resSz.WithLabelValues(status, c.Request.Method).Observe(resSz)
	}
}

// MetricsHandler returns an http.Handler that serves the metrics endpoint.
// Use this for net/http setups: http.Handle("/metrics", p.MetricsHandler())
func (p *Prometheus) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
}

func (p *Prometheus) prometheusHandler() gin.HandlerFunc {
	h := promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
// for the net/http middleware.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// From https://github.com/DanielHeckrath/gin-prometheus/blob/master/gin_prometheus.go
func computeApproximateRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s = len(r.URL.Path)
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	// N.B. r.Form and r.MultipartForm are assumed to be included in r.URL.
	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}
