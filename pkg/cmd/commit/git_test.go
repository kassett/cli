package commit

import (
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_makeRequest(t *testing.T) {
	host = "https://www.github.com/api/v3"
	repo, _ = ghrepo.FromFullName("OWNER/REPO")
	tests := []struct {
		name      string
		endpoint  string
		method    string
		body      map[string]interface{}
		data      interface{}
		want      map[string]interface{}
		expectErr bool
	}{
		{
			name:     "GET request without body",
			endpoint: "/test-get",
			method:   "GET",
			body:     nil,
			data:     nil,
			want: map[string]interface{}{
				"message": "GET request successful",
			},
			expectErr: false,
		},
		{
			name:     "POST request with body",
			endpoint: "/test-post",
			method:   "POST",
			body: map[string]interface{}{
				"key": "value",
			},
			data: nil,
			want: map[string]interface{}{
				"message": "POST request successful",
			},
			expectErr: false,
		},
		{
			name:     "GET request with data target",
			endpoint: "/test-get-data",
			method:   "GET",
			body:     nil,
			data:     &map[string]interface{}{},
			want: map[string]interface{}{
				"message": "GET request successful",
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			response, err := makeRequest(tt.endpoint, tt.method, tt.body, tt.data)

			// Assert
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, response)
			}
		})
	}
}
