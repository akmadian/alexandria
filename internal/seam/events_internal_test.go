package seam

import "testing"

// White-box tests for the envelope validation that gates every emit. buildEnvelope
// is the choke point WailsEmitter relies on to never push a malformed event.

func TestBuildEnvelope_ValidEventStampsTopicAndTimestamp(t *testing.T) {
	envelope, ok := buildEnvelope(EventCatalogChanged, CatalogChange{Scope: ScopeAssets})
	if !ok {
		t.Fatal("valid event should build")
	}
	if envelope.Topic != TopicCatalog {
		t.Fatalf("topic = %q, want %q (derived from the catalog, not the caller)", envelope.Topic, TopicCatalog)
	}
	if envelope.Type != EventCatalogChanged {
		t.Fatalf("type = %q", envelope.Type)
	}
	if envelope.Timestamp == "" {
		t.Fatal("timestamp should be stamped at emit time")
	}
}

func TestBuildEnvelope_UncatalogedTypeIsDropped(t *testing.T) {
	if _, ok := buildEnvelope(EventType("not_a_real_type"), CatalogChange{}); ok {
		t.Fatal("an uncataloged event type must not build")
	}
}

func TestBuildEnvelope_PayloadTypeMismatchIsDropped(t *testing.T) {
	// EventCatalogChanged expects a CatalogChange; handing it a JobProgress must be
	// rejected so a wrong-shaped payload can never cross the seam.
	if _, ok := buildEnvelope(EventCatalogChanged, JobProgress{}); ok {
		t.Fatal("a mismatched payload must not build")
	}
}

func TestValidateCatalog_RejectsBadCatalogs(t *testing.T) {
	cases := []struct {
		name    string
		catalog map[EventType]eventSpec
	}{
		{"unknown topic", map[EventType]eventSpec{"x": {Topic("bogus"), CatalogChange{}}}},
		{"nil payload", map[EventType]eventSpec{"x": {TopicCatalog, nil}}},
		{"duplicate payload", map[EventType]eventSpec{
			"x": {TopicCatalog, CatalogChange{}},
			"y": {TopicJobs, CatalogChange{}}, // same payload struct as x
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateCatalog(tc.catalog); err == nil {
				t.Fatalf("%s: expected a validation error, got nil", tc.name)
			}
		})
	}
}

func TestCatalogError_MessageNamesTypeAndReason(t *testing.T) {
	err := &catalogError{eventType: "someType", reason: "some reason"}
	if got := err.Error(); got == "" || !containsAll(got, "someType", "some reason") {
		t.Fatalf("error message %q should name the type and reason", got)
	}
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		found := false
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestValidateEventCatalog_CoversEveryDeclaredType(t *testing.T) {
	// Every EventType const must appear in the catalog. This asserts the ones this
	// round declares are present with their expected topics — the "declared type ⇒
	// a payload" half of C8 completeness, checked concretely.
	want := map[EventType]Topic{
		EventCatalogChanged: TopicCatalog,
		EventHistoryChanged: TopicCatalog,
		EventJobProgress:    TopicJobs,
		EventJobDone:        TopicJobs,
		EventVolumeStatus:   TopicWatcher,
	}
	for eventType, topic := range want {
		spec, ok := eventCatalog[eventType]
		if !ok {
			t.Errorf("event type %q missing from catalog", eventType)
			continue
		}
		if spec.topic != topic {
			t.Errorf("event %q topic = %q, want %q", eventType, spec.topic, topic)
		}
	}
	if len(eventCatalog) != len(want) {
		t.Errorf("catalog has %d entries, expected %d declared types", len(eventCatalog), len(want))
	}
}
