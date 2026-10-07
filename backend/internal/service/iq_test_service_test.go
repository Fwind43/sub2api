package service

import "testing"

func TestGradeQuestionSingleChoice(t *testing.T) {
	cases := []struct {
		name     string
		answer   string
		options  []string
		response string
		want     bool
	}{
		{"letter exact", "B", []string{"alpha", "beta", "gamma", "delta"}, "B", true},
		{"letter paren", "B", []string{"alpha", "beta"}, "B) beta", true},
		{"letter wrong", "B", []string{"alpha", "beta"}, "A", false},
		{"letter inside sentence", "C", []string{"a", "b", "c", "d"}, "The answer is C.", true},
		{"option text restated", "B", []string{"Paris", "Berlin"}, "Berlin", true},
		{"option text restated with prefix", "B", []string{"A) Paris", "B) Berlin"}, "B) Berlin", true},
		{"free text containment", "paris", nil, "The capital is Paris.", true},
		{"free text miss", "paris", nil, "The capital is Berlin.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &IQTestQuestion{Type: IQTestQuestionSingleChoice, Answer: tc.answer, Options: tc.options}
			if got := GradeQuestion(q, tc.response); got != tc.want {
				t.Fatalf("GradeQuestion=%v want %v", got, tc.want)
			}
		})
	}
}

func TestGradeQuestionOpen(t *testing.T) {
	q := &IQTestQuestion{
		Type:     IQTestQuestionOpen,
		Answer:   "gravity",
		Keywords: []string{"gravity", "mass", "acceleration"},
	}
	if !GradeQuestion(q, "Objects fall because of gravity acting on their mass.") {
		t.Fatal("expected keyword pass")
	}
	if GradeQuestion(q, "Completely unrelated answer about cooking.") {
		t.Fatal("expected keyword fail")
	}

	empty := &IQTestQuestion{Type: IQTestQuestionOpen}
	if !GradeQuestion(empty, "anything") {
		t.Fatal("empty answer config should pass on non-empty reply")
	}
	if GradeQuestion(empty, "   ") {
		t.Fatal("empty answer config should fail on blank reply")
	}
}

func TestNormalizeQuestions(t *testing.T) {
	qs := []*IQTestQuestion{{}}
	normalizeQuestions(qs)
	if qs[0].Type != IQTestQuestionSingleChoice || qs[0].Weight != 1 {
		t.Fatalf("normalize failed: %+v", qs[0])
	}
	if qs[0].Options == nil || qs[0].Keywords == nil {
		t.Fatal("slices should be non-nil")
	}
}
