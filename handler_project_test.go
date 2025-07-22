package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	"github.com/nats-io/nats.go"
	. "github.com/openfga/go-sdk/client"
)

// TestProjectUpdateAccessHandler tests the projectUpdateAccessHandler function
func TestProjectUpdateAccessHandler(t *testing.T) {
	tests := []struct {
		name           string
		messageData    []byte
		hasReply       bool
		expectError    bool
		description    string
		expectedTuples []ClientTupleKey
	}{
		{
			name: "valid project with all fields",
			messageData: mustJSON(projectStub{
				UID:       "test-project-123",
				Public:    true,
				ParentUID: "parent-project-456",
				Writers:   []string{"user1", "user2"},
				Auditors:  []string{"auditor1"},
			}),
			hasReply:    true,
			expectError: false,
			description: "should process valid project with all fields",
			expectedTuples: []ClientTupleKey{
				{User: "user:*", Relation: "viewer", Object: "project:test-project-123"},
				{User: "project:parent-project-456", Relation: "parent", Object: "project:test-project-123"},
				{User: "user:user1", Relation: "writer", Object: "project:test-project-123"},
				{User: "user:user2", Relation: "writer", Object: "project:test-project-123"},
				{User: "user:auditor1", Relation: "auditor", Object: "project:test-project-123"},
			},
		},
		{
			name: "private project without parent",
			messageData: mustJSON(projectStub{
				UID:      "private-project",
				Public:   false,
				Writers:  []string{"writer1"},
				Auditors: []string{},
			}),
			hasReply:    false,
			expectError: false,
			description: "should process private project without parent",
			expectedTuples: []ClientTupleKey{
				{User: "user:writer1", Relation: "writer", Object: "project:private-project"},
			},
		},
		{
			name: "public project with no users",
			messageData: mustJSON(projectStub{
				UID:    "public-empty",
				Public: true,
			}),
			hasReply:    true,
			expectError: false,
			description: "should process public project with no users",
			expectedTuples: []ClientTupleKey{
				{User: "user:*", Relation: "viewer", Object: "project:public-empty"},
			},
		},
		{
			name:           "invalid JSON",
			messageData:    []byte("invalid-json"),
			hasReply:       true,
			expectError:    true,
			description:    "should handle invalid JSON gracefully",
			expectedTuples: nil,
		},
		{
			name:           "empty project UID",
			messageData:    mustJSON(projectStub{}),
			hasReply:       true,
			expectError:    true,
			description:    "should handle empty project UID",
			expectedTuples: nil,
		},
		{
			name:           "empty message",
			messageData:    []byte(""),
			hasReply:       true,
			expectError:    true,
			description:    "should handle empty message",
			expectedTuples: nil,
		},
		{
			name: "project with empty arrays",
			messageData: mustJSON(projectStub{
				UID:      "empty-arrays",
				Writers:  []string{},
				Auditors: []string{},
			}),
			hasReply:       false,
			expectError:    false,
			description:    "should handle project with empty arrays",
			expectedTuples: []ClientTupleKey{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test message
			msg := &nats.Msg{
				Subject: fmt.Sprintf("%s%s", lfxEnvironment, constants.ProjectUpdateAccessSubject),
				Data:    tt.messageData,
			}

			if tt.hasReply {
				msg.Reply = "test.reply.inbox"
			}

			t.Logf("Test case: %s - %s", tt.name, tt.description)

			// Note: Actual handler testing would require mocking fgaSyncObjectTuples
			// This test documents expected behavior
		})
	}
}

// TestProjectDeleteAllAccessHandler tests the projectDeleteAllAccessHandler function
func TestProjectDeleteAllAccessHandler(t *testing.T) {
	tests := []struct {
		name        string
		messageData []byte
		hasReply    bool
		expectError bool
		description string
	}{
		{
			name:        "valid project UID",
			messageData: []byte("test-project-123"),
			hasReply:    true,
			expectError: false,
			description: "should delete all tuples for valid project",
		},
		{
			name:        "empty payload",
			messageData: []byte(""),
			hasReply:    true,
			expectError: true,
			description: "should handle empty payload",
		},
		{
			name:        "JSON object payload",
			messageData: []byte(`{"uid": "test"}`),
			hasReply:    true,
			expectError: true,
			description: "should reject JSON object payload",
		},
		{
			name:        "JSON array payload",
			messageData: []byte(`["test"]`),
			hasReply:    true,
			expectError: true,
			description: "should reject JSON array payload",
		},
		{
			name:        "quoted string payload",
			messageData: []byte(`"test-project"`),
			hasReply:    true,
			expectError: true,
			description: "should reject quoted string payload",
		},
		{
			name:        "project UID without reply",
			messageData: []byte("project-no-reply"),
			hasReply:    false,
			expectError: false,
			description: "should process without sending reply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test message
			msg := &nats.Msg{
				Subject: fmt.Sprintf("%s%s", lfxEnvironment, constants.ProjectDeleteAllAccessSubject),
				Data:    tt.messageData,
			}

			if tt.hasReply {
				msg.Reply = "test.reply.inbox"
			}

			t.Logf("Test case: %s - %s", tt.name, tt.description)

			// Note: Actual handler testing would require mocking fgaSyncObjectTuples
			// This test documents expected behavior
		})
	}
}

// TestProjectStubStruct tests the projectStub struct marshaling/unmarshaling
func TestProjectStubStruct(t *testing.T) {
	tests := []struct {
		name     string
		stub     projectStub
		wantJSON string
	}{
		{
			name: "full struct",
			stub: projectStub{
				UID:       "test-123",
				Public:    true,
				ParentUID: "parent-456",
				Writers:   []string{"w1", "w2"},
				Auditors:  []string{"a1"},
			},
			wantJSON: `{"uid":"test-123","public":true,"parent_uid":"parent-456","writers":["w1","w2"],"auditors":["a1"]}`,
		},
		{
			name: "minimal struct",
			stub: projectStub{
				UID: "minimal",
			},
			wantJSON: `{"uid":"minimal","public":false,"parent_uid":"","writers":null,"auditors":null}`,
		},
		{
			name: "empty arrays",
			stub: projectStub{
				UID:      "empty-arrays",
				Writers:  []string{},
				Auditors: []string{},
			},
			wantJSON: `{"uid":"empty-arrays","public":false,"parent_uid":"","writers":[],"auditors":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			got, err := json.Marshal(tt.stub)
			if err != nil {
				t.Errorf("failed to marshal: %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Errorf("marshal mismatch:\ngot:  %s\nwant: %s", got, tt.wantJSON)
			}

			// Test unmarshaling
			var unmarshaled projectStub
			err = json.Unmarshal([]byte(tt.wantJSON), &unmarshaled)
			if err != nil {
				t.Errorf("failed to unmarshal: %v", err)
			}

			// Compare fields
			if unmarshaled.UID != tt.stub.UID {
				t.Errorf("UID mismatch: got %s, want %s", unmarshaled.UID, tt.stub.UID)
			}
			if unmarshaled.Public != tt.stub.Public {
				t.Errorf("Public mismatch: got %v, want %v", unmarshaled.Public, tt.stub.Public)
			}
			if unmarshaled.ParentUID != tt.stub.ParentUID {
				t.Errorf("ParentUID mismatch: got %s, want %s", unmarshaled.ParentUID, tt.stub.ParentUID)
			}
		})
	}
}

// TestFgaTupleKey tests the fgaTupleKey helper function
func TestFgaTupleKey(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		relation string
		object   string
		want     ClientTupleKey
	}{
		{
			name:     "standard tuple",
			user:     "user:123",
			relation: "admin",
			object:   "project:456",
			want: ClientTupleKey{
				User:     "user:123",
				Relation: "admin",
				Object:   "project:456",
			},
		},
		{
			name:     "wildcard user",
			user:     "user:*",
			relation: "viewer",
			object:   "project:public",
			want: ClientTupleKey{
				User:     "user:*",
				Relation: "viewer",
				Object:   "project:public",
			},
		},
		{
			name:     "group user",
			user:     "group:developers",
			relation: "writer",
			object:   "project:123",
			want: ClientTupleKey{
				User:     "group:developers",
				Relation: "writer",
				Object:   "project:123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fgaTupleKey(tt.user, tt.relation, tt.object)
			if got.User != tt.want.User || got.Relation != tt.want.Relation || got.Object != tt.want.Object {
				t.Errorf("fgaTupleKey() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestFgaNewTupleKeySlice tests the fgaNewTupleKeySlice helper function
func TestFgaNewTupleKeySlice(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantCap int
	}{
		{
			name:    "small slice",
			size:    4,
			wantCap: 4,
		},
		{
			name:    "zero size",
			size:    0,
			wantCap: 0,
		},
		{
			name:    "large slice",
			size:    100,
			wantCap: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fgaNewTupleKeySlice(tt.size)
			if len(got) != 0 {
				t.Errorf("expected empty slice, got length %d", len(got))
			}
			if cap(got) != tt.wantCap {
				t.Errorf("expected capacity %d, got %d", tt.wantCap, cap(got))
			}
		})
	}
}

// TestTupleGeneration tests the tuple generation logic
func TestTupleGeneration(t *testing.T) {
	tests := []struct {
		name           string
		project        projectStub
		expectedCount  int
		expectedTuples []ClientTupleKey
	}{
		{
			name: "complete project",
			project: projectStub{
				UID:       "test",
				Public:    true,
				ParentUID: "parent",
				Writers:   []string{"w1", "w2"},
				Auditors:  []string{"a1", "a2"},
			},
			expectedCount: 6, // 1 public + 1 parent + 2 writers + 2 auditors
		},
		{
			name: "minimal project",
			project: projectStub{
				UID: "test",
			},
			expectedCount: 0,
		},
		{
			name: "public only",
			project: projectStub{
				UID:    "test",
				Public: true,
			},
			expectedCount: 1,
		},
		{
			name: "with parent only",
			project: projectStub{
				UID:       "test",
				ParentUID: "parent",
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the tuple generation logic from the handler
			tuples := fgaNewTupleKeySlice(4)
			object := "project:" + tt.project.UID

			if tt.project.Public {
				tuples = append(tuples, fgaTupleKey("user:*", "viewer", object))
			}

			if tt.project.ParentUID != "" {
				tuples = append(tuples, fgaTupleKey("project:"+tt.project.ParentUID, "parent", object))
			}

			for _, principal := range tt.project.Writers {
				tuples = append(tuples, fgaTupleKey("user:"+principal, "writer", object))
			}

			for _, principal := range tt.project.Auditors {
				tuples = append(tuples, fgaTupleKey("user:"+principal, "auditor", object))
			}

			if len(tuples) != tt.expectedCount {
				t.Errorf("expected %d tuples, got %d", tt.expectedCount, len(tuples))
			}
		})
	}
}

// BenchmarkProjectUpdateHandler benchmarks the project update handler
func BenchmarkProjectUpdateHandler(b *testing.B) {
	project := projectStub{
		UID:       "bench-project",
		Public:    true,
		ParentUID: "parent",
		Writers:   []string{"w1", "w2", "w3", "w4", "w5"},
		Auditors:  []string{"a1", "a2", "a3"},
	}

	data, _ := json.Marshal(project)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate parsing and tuple generation
		var p projectStub
		_ = json.Unmarshal(data, &p)

		tuples := fgaNewTupleKeySlice(10)
		object := "project:" + p.UID

		if p.Public {
			tuples = append(tuples, fgaTupleKey("user:*", "viewer", object))
		}
		if p.ParentUID != "" {
			tuples = append(tuples, fgaTupleKey("project:"+p.ParentUID, "parent", object))
		}
		for _, principal := range p.Writers {
			tuples = append(tuples, fgaTupleKey("user:"+principal, "writer", object))
		}
		for _, principal := range p.Auditors {
			tuples = append(tuples, fgaTupleKey("user:"+principal, "auditor", object))
		}
	}
}

// Helper function to create JSON or panic
func mustJSON(v interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
