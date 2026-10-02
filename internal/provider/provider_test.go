package provider

import (
	"testing"
)

func TestBuildAuthProvider(t *testing.T) {
	type args struct {
		authMethod             string
		username               string
		password               string
		oauthToken             string
		oauthClientID          string
		oauthClientSecret      string
		oauthServerURI         string
		oauthTokenPath         string
		oauthScope             string
		kerberosPrincipal      string
		kerberosKeytab         string
		kerberosUseTicketCache bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		wantNil bool
	}{
		{
			name:    "empty auth defaults to nil",
			args:    args{authMethod: ""},
			wantErr: false,
			wantNil: true,
		},
		{
			name:    "none auth defaults to nil",
			args:    args{authMethod: "none"},
			wantErr: false,
			wantNil: true,
		},
		{
			name:    "simple",
			args:    args{authMethod: "simple"},
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "basic with username",
			args:    args{authMethod: "basic", username: "testuser"},
			wantErr: false,
			wantNil: false,
		},
		{
			name:    "basic without username",
			args:    args{authMethod: "basic"},
			wantErr: true,
			wantNil: true,
		},
		{
			name:    "unknown method",
			args:    args{authMethod: "invalid"},
			wantErr: true,
			wantNil: true,
		},
		{
			name:    "kerberos without principal",
			args:    args{authMethod: "kerberos"},
			wantErr: true,
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.args
			ap, err := buildAuthProvider(a.authMethod, a.username, a.password,
				a.oauthToken, a.oauthClientID, a.oauthClientSecret,
				a.oauthServerURI, a.oauthTokenPath, a.oauthScope,
				a.kerberosPrincipal, a.kerberosKeytab, a.kerberosUseTicketCache)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil && ap != nil {
				t.Fatal("expected nil provider")
			}
			if !tt.wantNil && ap == nil && !tt.wantErr {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}
