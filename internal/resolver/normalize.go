package resolver

import "github.com/transpose-project/transpose/internal/resolver/repology"

func CandidateNames(source string) []string {
	return repology.CandidateProjectNames(source)
}
