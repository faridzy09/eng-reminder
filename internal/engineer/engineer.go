package engineer

import "strings"

// Engineer represents a data engineer team member.
type Engineer struct {
	ID                int
	Name              string
	StoryPointsPerDay int
	Supervisor        string
}

// Team is the list of all data engineers.
// Yandra, M. Arif, and Fadli have 8 SP/day; all others have 6 SP/day.
var Team = []Engineer{
	{ID: 1, Name: "Yandra Charlos Hasugian", StoryPointsPerDay: 8, Supervisor: "DeriKurniawan"},
	{ID: 2, Name: "Fiqri Ramadhan", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 3, Name: "Muhamad Lutfi Alfiansyah", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 4, Name: "Adi Saputra", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 5, Name: "Andika Prasetya", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 6, Name: "Naufal Hadi", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 7, Name: "Fuad Rifqi Zamzami", StoryPointsPerDay: 8, Supervisor: "Faridho"},
	{ID: 8, Name: "Andikha Apriadi", StoryPointsPerDay: 6, Supervisor: "Sholahuddin Alisyahbana"},
	{ID: 9, Name: "Junifer Rionaldi Manik", StoryPointsPerDay: 6, Supervisor: "Susi Cahyati"},
	{ID: 10, Name: "M. Arif Sefrianto", StoryPointsPerDay: 8, Supervisor: "Irvan Resna Hadiyana"},
	{ID: 11, Name: "Fadli Muhamad Paridi", StoryPointsPerDay: 8, Supervisor: "Susi Cahyati"},
	{ID: 12, Name: "Anom Yulian Hartanto", StoryPointsPerDay: 6, Supervisor: "Sholahuddin Alisyahbana"},
	{ID: 13, Name: "Yusuf Gutara", StoryPointsPerDay: 6, Supervisor: "Muhammad Farid H"},
	{ID: 14, Name: "Fajrul Aulia", StoryPointsPerDay: 6, Supervisor: "Sholahuddin Alisyahbana"},
	{ID: 15, Name: "Dani Mulyana", StoryPointsPerDay: 6, Supervisor: "Irvan Resna Hadiyana"},
	{ID: 16, Name: "Rosyid Rosadi", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 17, Name: "Fajar Darwis", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 18, Name: "Rifat Firdaus", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 19, Name: "Clara Anggraini", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
	{ID: 20, Name: "Indra Ikwal", StoryPointsPerDay: 6, Supervisor: "DeriKurniawan"},
	{ID: 21, Name: "Pratama Egho", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 22, Name: "Faizal Bima", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 23, Name: "Abdul Ghani Abbasi", StoryPointsPerDay: 6, Supervisor: "Sholahuddin Alisyahbana"},
	{ID: 24, Name: "Nicholas Fortune", StoryPointsPerDay: 6, Supervisor: "Sholahuddin Alisyahbana"},
	{ID: 25, Name: "Dorojatun Chandrabumi", StoryPointsPerDay: 6, Supervisor: "Faridho"},
	{ID: 26, Name: "Ridho Tanjung", StoryPointsPerDay: 6, Supervisor: "Falih Mulyana"},
}

// NamesBySupervisor returns the names of all engineers reporting to the given
// supervisor. Used to build Jira assignee filters per lead (e.g. FE team).
func NamesBySupervisor(supervisor string) []string {
	names := []string{}
	for i := range Team {
		if Team[i].Supervisor == supervisor {
			names = append(names, Team[i].Name)
		}
	}
	return names
}

// FindByName returns an engineer by name (case-insensitive contains match).
// Returns nil if not found.
func FindByName(name string) *Engineer {
	nameLower := toLower(name)
	for i := range Team {
		if toLower(Team[i].Name) == nameLower {
			return &Team[i]
		}
	}
	return nil
}

// FindByJiraDisplayName returns an engineer whose name best matches the given
// Jira assignee displayName using case-insensitive word matching.
// Returns nil if no match is found.
func FindByJiraDisplayName(displayName string) *Engineer {
	if displayName == "" || displayName == "_Unassigned_" {
		return nil
	}
	displayLower := strings.ToLower(strings.TrimSpace(displayName))

	// 1. exact match
	for i := range Team {
		if strings.ToLower(Team[i].Name) == displayLower {
			return &Team[i]
		}
	}

	// 2. all meaningful words (len > 2) in displayName must appear in engineer name
	words := strings.Fields(displayLower)
	meaningful := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 2 {
			meaningful = append(meaningful, w)
		}
	}
	if len(meaningful) == 0 {
		return nil
	}
	for i := range Team {
		nameLower := strings.ToLower(Team[i].Name)
		allMatch := true
		for _, w := range meaningful {
			if !strings.Contains(nameLower, w) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return &Team[i]
		}
	}
	return nil
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}
