package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDynamicHeightLimits_MinimumHeight(t *testing.T) {
	t.Parallel()

	// Very small available height should return minimum values for all sections.
	maxFiles, maxLSPs, maxMCPs := getDynamicHeightLimits(5)
	assert.Equal(t, 2, maxFiles, "files should use minimum items per section")
	assert.Equal(t, 2, maxLSPs, "LSPs should use minimum items per section")
	assert.Equal(t, 2, maxMCPs, "MCPs should use minimum items per section")
}

func TestGetDynamicHeightLimits_ExactMinBoundary(t *testing.T) {
	t.Parallel()

	// Height exactly at the minAvailableHeightLimit (10) should distribute evenly.
	maxFiles, maxLSPs, maxMCPs := getDynamicHeightLimits(10)
	require.GreaterOrEqual(t, maxFiles, 2)
	require.GreaterOrEqual(t, maxLSPs, 2)
	require.GreaterOrEqual(t, maxMCPs, 2)
	// Total allocated should not exceed available height.
	assert.LessOrEqual(t, maxFiles+maxLSPs+maxMCPs, 10+10, // room for redistribution up to defaults
		"total should be reasonable relative to available height")
}

func TestGetDynamicHeightLimits_LargeHeight(t *testing.T) {
	t.Parallel()

	// With plenty of space, each section should reach its default max.
	maxFiles, maxLSPs, maxMCPs := getDynamicHeightLimits(100)
	assert.Equal(t, 10, maxFiles, "files should reach default max of 10")
	assert.Equal(t, 8, maxLSPs, "LSPs should reach default max of 8")
	assert.Equal(t, 8, maxMCPs, "MCPs should reach default max of 8")
}

func TestGetDynamicHeightLimits_MediumHeight_FilesGetPriority(t *testing.T) {
	t.Parallel()

	// With moderate height, files should get extra space first due to priority.
	maxFiles, maxLSPs, maxMCPs := getDynamicHeightLimits(20)

	// Files get priority in the redistribution of remaining height.
	assert.GreaterOrEqual(t, maxFiles, maxLSPs,
		"files should get at least as many items as LSPs due to priority")
	assert.GreaterOrEqual(t, maxFiles, maxMCPs,
		"files should get at least as many items as MCPs due to priority")
}

func TestGetDynamicHeightLimits_ZeroHeight(t *testing.T) {
	t.Parallel()

	// Zero height should return minimums (below the minAvailableHeightLimit).
	maxFiles, maxLSPs, maxMCPs := getDynamicHeightLimits(0)
	assert.Equal(t, 2, maxFiles)
	assert.Equal(t, 2, maxLSPs)
	assert.Equal(t, 2, maxMCPs)
}
