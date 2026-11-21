package completiongates

import "expvar"

// Expvar counters with closed label sets. No task or criterion identifiers.
var (
	evaluationTotal      = expvar.NewMap("office_completion_gate_evaluation_total")
	holdTotal            = expvar.NewMap("office_completion_gate_hold_total")
	reworkTotal          = expvar.NewMap("office_completion_gate_rework_total")
	verifierRefusedTotal = expvar.NewMap("office_completion_gate_verifier_refused_total")
)

func recordEvaluationMetric(kind, result string) { evaluationTotal.Add(kind+"|"+result, 1) }

func recordHoldMetric(reason string) { holdTotal.Add(reason, 1) }

func recordReworkMetric(kind string) { reworkTotal.Add(kind, 1) }

// RecordVerifierRefused counts a refused or ignored verification.
func RecordVerifierRefused(reason string) { verifierRefusedTotal.Add(reason, 1) }
