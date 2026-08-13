// Package model holds the plain data types shared between the store and web
// layers. No behavior lives here beyond small derived-field helpers.
package model

import "fmt"

type Project struct {
	ID              int64
	KeyPrefix       string
	Name            string
	Description     string
	NextIssueNumber int
	CreatedAt       string
	UpdatedAt       string
}

type IssueType struct {
	ID        int64
	Name      string
	Color     string
	Icon      string
	NoSprint  bool
	IsDefault bool
}

type Status struct {
	ID        int64
	Name      string
	Category  string // todo | in_progress | done
	SortOrder int
}

type Sprint struct {
	ID        int64
	ProjectID int64
	Name      string
	Goal      string
	StartDate string
	EndDate   string
	State     string // planned | active | completed
}

// Issue is the core entity, denormalized with the joined fields templates
// need so handlers don't have to stitch lookups together themselves.
type Issue struct {
	ID          int64
	ProjectID   int64
	IssueNumber int
	IssueTypeID int64
	ParentID    *int64
	Summary     string
	Description string
	StatusID    int64
	Priority    string
	StoryPoints *int
	DueDate     string
	SprintID    *int64
	Position    float64
	CreatedAt   string
	UpdatedAt   string

	KeyPrefix      string
	TypeName       string
	TypeColor      string
	TypeIcon       string
	TypeNoSprint   bool
	StatusName     string
	StatusCategory string
	SprintName     string
	Labels         []string
	Components     []string
}

// Key is the computed, never-stored issue key (e.g. "WEB-42").
func (i Issue) Key() string {
	return fmt.Sprintf("%s-%d", i.KeyPrefix, i.IssueNumber)
}

type Comment struct {
	ID        int64
	IssueID   int64
	Body      string
	CreatedAt string
	UpdatedAt string
}

type IssueLink struct {
	ID            int64
	SourceIssueID int64
	TargetIssueID int64
	LinkType      string // blocks | relates_to

	// Populated for display/linking: the *other* issue in the link.
	// OtherKey is the formatted "WEB-5" for display; OtherNumber is the
	// bare issue number the /issues/{num} route actually expects.
	OtherKey     string
	OtherNumber  int
	OtherSummary string
}

// SprintIssue is a row from the sprint membership history table.
type SprintIssue struct {
	ID                      int64
	SprintID                int64
	IssueID                 int64
	AddedAt                 string
	RemovedAt               *string
	StatusCategoryAtRemoval *string

	Issue Issue
}
