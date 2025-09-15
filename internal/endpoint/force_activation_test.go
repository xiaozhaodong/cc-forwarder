package endpoint

import (
	"testing"
	"time"

	"cc-forwarder/config"
)

func TestForceActivation(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Group: config.GroupConfig{
			Cooldown:                time.Minute,
			AutoSwitchBetweenGroups: false,
		},
	}

	// Create group manager
	gm := NewGroupManager(cfg)

	// Create test endpoints
	endpoints := []*Endpoint{
		{
			Config: config.EndpointConfig{
				Name:     "healthy-1",
				URL:      "https://api.example.com",
				Group:    "test-group",
				GroupPriority: 1,
			},
			Status: EndpointStatus{
				Healthy: true,
			},
		},
		{
			Config: config.EndpointConfig{
				Name:     "unhealthy-1",
				URL:      "https://invalid.example.com",
				Group:    "test-group",
				GroupPriority: 1,
			},
			Status: EndpointStatus{
				Healthy: false,
			},
		},
	}

	gm.UpdateGroups(endpoints)

	t.Run("Normal activation with healthy endpoints", func(t *testing.T) {
		err := gm.ManualActivateGroup("test-group")
		if err != nil {
			t.Errorf("Normal activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		if len(groups) == 0 || !groups[0].IsActive {
			t.Error("Group should be active after normal activation")
		}
		if groups[0].ForcedActivation {
			t.Error("Group should not be marked as force activated")
		}
	})

	t.Run("Normal activation fails with no healthy endpoints", func(t *testing.T) {
		// Make all endpoints unhealthy
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroup("test-group")
		if err == nil {
			t.Error("Normal activation should fail with no healthy endpoints")
		}
		if err.Error() != "组 test-group 中没有健康的端点，无法激活。如需强制激活请使用强制模式" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Force activation succeeds with no healthy endpoints", func(t *testing.T) {
		// Ensure all endpoints are unhealthy
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroupWithForce("test-group", true)
		if err != nil {
			t.Errorf("Force activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		if len(groups) == 0 || !groups[0].IsActive {
			t.Error("Group should be active after force activation")
		}
		if !groups[0].ForcedActivation {
			t.Error("Group should be marked as force activated")
		}
		if groups[0].ForcedActivationTime.IsZero() {
			t.Error("Force activation time should be set")
		}
	})

	t.Run("Force activation fails with healthy endpoints", func(t *testing.T) {
		// Make one endpoint healthy
		endpoints[0].Status.Healthy = true
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroupWithForce("test-group", true)
		if err == nil {
			t.Error("Force activation should fail with healthy endpoints")
		}
		if err.Error() != "组 test-group 有 1 个健康端点，无需强制激活。请使用正常激活" {
			t.Errorf("Expected specific error message, got: %v", err)
		}
	})

	t.Run("Group details include force activation info", func(t *testing.T) {
		// Force activate with no healthy endpoints
		endpoints[0].Status.Healthy = false
		endpoints[1].Status.Healthy = false
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroupWithForce("test-group", true)
		if err != nil {
			t.Errorf("Force activation failed: %v", err)
		}

		details := gm.GetGroupDetails()
		groups, ok := details["groups"].([]map[string]interface{})
		if !ok || len(groups) == 0 {
			t.Fatal("Expected group details")
		}

		group := groups[0]
		if group["forced_activation"] != true {
			t.Error("Group should be marked as force activated in details")
		}
		if group["activation_type"] != "forced" {
			t.Error("Activation type should be 'forced'")
		}
		if group["can_force_activate"] != false {
			t.Error("Can force activate should be false for active group")
		}
		if forcedTime, ok := group["forced_activation_time"].(string); !ok || forcedTime == "" {
			t.Error("Forced activation time should be set")
		}
	})

	t.Run("Normal activation clears force activation flags", func(t *testing.T) {
		// Make endpoints healthy and do normal activation
		endpoints[0].Status.Healthy = true
		endpoints[1].Status.Healthy = true
		gm.UpdateGroups(endpoints)

		err := gm.ManualActivateGroup("test-group")
		if err != nil {
			t.Errorf("Normal activation failed: %v", err)
		}

		groups := gm.GetAllGroups()
		if len(groups) == 0 || !groups[0].IsActive {
			t.Error("Group should be active after normal activation")
		}
		if groups[0].ForcedActivation {
			t.Error("Force activation flag should be cleared")
		}
		if !groups[0].ForcedActivationTime.IsZero() {
			t.Error("Force activation time should be cleared")
		}
	})
}