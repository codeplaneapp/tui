package attachments

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRenderer() *Renderer {
	normal := lipgloss.NewStyle()
	deleting := lipgloss.NewStyle()
	img := lipgloss.NewStyle().SetString("IMG")
	text := lipgloss.NewStyle().SetString("TXT")
	return NewRenderer(normal, deleting, img, text)
}

func TestIcon_ImageAttachment(t *testing.T) {
	t.Parallel()
	r := newTestRenderer()

	imgAtt := message.Attachment{FileName: "photo.png", MimeType: "image/png"}
	txtAtt := message.Attachment{FileName: "notes.txt", MimeType: "text/plain"}

	imgIcon := r.icon(imgAtt)
	txtIcon := r.icon(txtAtt)

	// The image style has SetString("IMG"), the text style has SetString("TXT").
	assert.Equal(t, "IMG", imgIcon.String(), "image attachment should use image style")
	assert.Equal(t, "TXT", txtIcon.String(), "text attachment should use text style")
}

func TestRender_EmptyAttachments(t *testing.T) {
	t.Parallel()
	r := newTestRenderer()

	got := r.Render(nil, false, 120)
	assert.Empty(t, got, "rendering nil attachments should produce empty string")
}

func TestRender_SingleAttachment(t *testing.T) {
	t.Parallel()
	r := newTestRenderer()

	attachments := []message.Attachment{
		{FileName: "readme.md", MimeType: "text/markdown"},
	}

	got := r.Render(attachments, false, 120)
	require.NotEmpty(t, got)
	assert.Contains(t, got, "readme.md", "output should contain the filename")
}
