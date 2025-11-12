package uploader

import "testing"

func TestValidateThumbnailSizes(t *testing.T) {
	cases := []struct {
		name      string
		sizes     []ThumbnailSize
		expectErr bool
	}{
		{
			name: "valid sizes",
			sizes: []ThumbnailSize{
				{Name: "small", Width: 100, Height: 100, Fit: "cover"},
				{Name: "medium", Width: 300, Height: 200, Fit: "contain"},
			},
			expectErr: false,
		},
		{
			name:      "empty list",
			sizes:     nil,
			expectErr: true,
		},
		{
			name: "duplicate name",
			sizes: []ThumbnailSize{
				{Name: "Small", Width: 100, Height: 100, Fit: "cover"},
				{Name: "small", Width: 200, Height: 200, Fit: "cover"},
			},
			expectErr: true,
		},
		{
			name: "invalid width",
			sizes: []ThumbnailSize{
				{Name: "bad", Width: 0, Height: 100, Fit: "cover"},
			},
			expectErr: true,
		},
		{
			name: "invalid fit",
			sizes: []ThumbnailSize{
				{Name: "bad-fit", Width: 100, Height: 100, Fit: "stretch"},
			},
			expectErr: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateThumbnailSizes(tc.sizes)
			if tc.expectErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("expected no error but got %v", err)
			}
		})
	}
}
