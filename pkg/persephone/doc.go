// Package persephone implements seasonal scaling and predictive forecasting —
// the Queen of Cycles who governs the rhythms of demand across time.
//
// Named after the goddess who alternates between the Underworld and the
// surface world in a seasonal cycle, Persephone models recurring patterns
// in sandbox demand and proactively adjusts cluster capacity.
//
// # Core Types
//
//   - [SeasonalScaler]: The main interface. Provides Forecast, DefineSeason,
//     ApplySeason, Learn, CurrentSeason, and RecommendCapacity.
//
//   - [Season]: A time-bounded scaling configuration with min/max node counts,
//     target utilisation, pre-warming parameters, resource mix, and budget
//     constraints.
//
//   - [PatternDetector]: Analyses historical [UsageRecord] data to detect
//     hourly and daily demand patterns. Generates confidence-weighted
//     forecasts using MSE-based error estimation.
//
//   - [BudgetTracker]: Monitors daily and monthly cost against configured
//     limits. Can enforce a hard cap that prevents scaling beyond budget.
//
//   - [CapacityOptimizer]: Recommends optimal node count given target
//     utilisation and historical demand.
//
//   - [CronScheduler]: Evaluates cron-style season schedule expressions and
//     activates the appropriate season at runtime.
//
//   - [PrometheusCollector]: Pulls historical usage data from Prometheus to
//     feed the [PatternDetector].
//
// # Sub-package: evaluator
//
//   - [persephone/evaluator]: Backtests forecast accuracy using MAPE, RMSE,
//     prediction interval coverage, and bias metrics. Produces an
//     [EvaluationReport] for model health monitoring.
//
// # Known Technical Debt
//
//   - The cron parser ([CronScheduler]) is hand-rolled and only supports a
//     limited subset of cron syntax (no @monthly, @weekly shortcuts, no
//     L/W/# modifiers). A production system should use a well-tested library
//     such as github.com/robfig/cron.
//
//   - [PrometheusCollector] Prometheus query warnings are silently discarded.
//     Warnings about partial data (e.g., during federation) could lead to
//     incorrect pattern detection.
//
//   - The demand model uses a linear combination of hourly and daily buckets.
//     It does not model non-linear spikes (e.g., viral traffic events) or
//     decay effects. A more sophisticated model (ARIMA, Prophet) is planned
//     but not implemented.
//
//   - Seasons are stored in-memory only. Defined seasons are lost on Olympus
//     restart unless re-registered via the API.
//
//   - The idle-node hibernation policy ([hibernation.go]) is implemented but
//     not wired to [hypnos.Manager]. Hibernate commands are logged but not
//     actually sent to the agent.
package persephone
