package integrity

import (
	"math"
)

type SlotRecord struct {
	Slot      string `json:"slot"`
	Status    string `json:"status"`
	Validator string `json:"validator,omitempty"`
}

type EpochAnalysisInput struct {
	EpochNumber                int64
	Records                    []SlotRecord
	ExpectedValidatorsPerEpoch int
	SlotsPerEpoch              int
}

type EpochIntegrityResult struct {
	EpochNumber                int64
	TotalValidators            int
	BlockMissedCount           int
	BlockMinedCount            int
	BlockProposedCount         int
	AttestationSentCount       int
	AttestationMissedCount     int
	EmptyValidators            int
	Issues                     []string
	ExpectedValidatorsPerEpoch int
	IntegrityScore             int
	IntegrityStatus            string
}

func AnalyzeEpochIntegrity(input EpochAnalysisInput) EpochIntegrityResult {
	// Count unique validators
	validators := make(map[string]struct{})
	for _, r := range input.Records {
		if r.Validator != "" {
			validators[r.Validator] = struct{}{}
		}
	}
	totalValidators := len(validators)

	// Count statuses
	counts := make(map[string]int)
	for _, r := range input.Records {
		counts[r.Status]++
	}

	blockMissed := counts["block-missed"]
	blockMined := counts["block-mined"]
	blockProposed := counts["block-proposed"]
	attEstationSent := counts["attestation-sent"]
	attestationMissed := counts["attestation-missed"]

	emptyValidators := input.ExpectedValidatorsPerEpoch - totalValidators
	if emptyValidators < 0 {
		emptyValidators = 0
	}

	result := EpochIntegrityResult{
		EpochNumber:                input.EpochNumber,
		TotalValidators:            totalValidators,
		BlockMissedCount:           blockMissed,
		BlockMinedCount:            blockMined,
		BlockProposedCount:         blockProposed,
		AttestationSentCount:       attEstationSent,
		AttestationMissedCount:     attestationMissed,
		EmptyValidators:            emptyValidators,
		Issues:                     []string{},
		ExpectedValidatorsPerEpoch: input.ExpectedValidatorsPerEpoch,
		IntegrityScore:             0,
		IntegrityStatus:            "UNKNOWN",
	}

	score := 100.0

	// Rule 1: Check total unique validators
	if totalValidators > input.ExpectedValidatorsPerEpoch {
		result.Issues = append(result.Issues, "Has too many unique validators") // Simplified msg
		score -= 40
	}

	// Rule 2: Check total block records
	totalBlockRecords := blockMined + blockProposed + blockMissed
	if totalBlockRecords != input.SlotsPerEpoch {
		diff := math.Abs(float64(totalBlockRecords - input.SlotsPerEpoch))
		penalty := (diff / float64(input.SlotsPerEpoch)) * 30
		result.Issues = append(result.Issues, "Block record count mismatch")
		score -= penalty
	}

	// Rule 3: Check attestation record distribution
	totalAttestations := attEstationSent + attestationMissed
	slotsWithBlocks := blockMined + blockProposed
	expectedAttestations := slotsWithBlocks * (input.ExpectedValidatorsPerEpoch - 1)

	if totalAttestations != expectedAttestations && expectedAttestations > 0 {
		diff := math.Abs(float64(totalAttestations - expectedAttestations))
		maxDiff := float64(input.SlotsPerEpoch * input.ExpectedValidatorsPerEpoch)
		penalty := (diff / maxDiff) * 25
		result.Issues = append(result.Issues, "Attestation count mismatch")
		score -= penalty
	}

	// Rule 4: Check empty validators
	if blockMissed == 0 && emptyValidators > 0 {
		penalty := (float64(emptyValidators) / float64(input.ExpectedValidatorsPerEpoch)) * 20
		result.Issues = append(result.Issues, "Has empty validators with no block-missed")
		score -= penalty
	}

	finalScore := int(math.Max(0, math.Round(score)))
	result.IntegrityScore = finalScore

	if finalScore == 100 {
		result.IntegrityStatus = "VALID"
	} else if finalScore >= 90 {
		result.IntegrityStatus = "WARNING"
	} else if finalScore >= 50 {
		result.IntegrityStatus = "PARTIAL"
	} else {
		result.IntegrityStatus = "INVALID"
	}

	return result
}
