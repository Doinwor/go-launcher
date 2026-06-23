package launch

import (
	"testing"
)

func TestParseJavaVersion(t *testing.T) {
	tests := []struct {
		output  string
		want    int
		wantErr bool
	}{
		{`openjdk version "17.0.8" 2023-07-18`, 17, false},
		{`openjdk version "21.0.1" 2023-10-17`, 21, false},
		{`java version "1.8.0_202"`, 8, false},
		{`java version "11.0.20" 2023-07-18`, 11, false},
		{`openjdk version "1.7.0_80"`, 7, false},
		{`java version "22-ea" 2024-03-19`, 22, false},
		{`not a java version string at all`, 0, true},
		{``, 0, true},
		{`java version "invalid"`, 0, true},
	}

	for _, tc := range tests {
		got, err := ParseJavaVersion(tc.output)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseJavaVersion(%q) expected error, got %d", tc.output, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseJavaVersion(%q) unexpected error: %v", tc.output, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseJavaVersion(%q) = %d, want %d", tc.output, got, tc.want)
		}
	}
}

func TestParseJavaVersionRealOutput(t *testing.T) {
	// Simulate real java -version output for common JVM distributions
	realOutputs := []struct {
		output string
		want   int
	}{
		{
			output: `openjdk version "17.0.9" 2023-10-17 LTS
OpenJDK Runtime Environment Temurin-17.0.9+9 (build 17.0.9+9)
OpenJDK 64-Bit Server VM Temurin-17.0.9+9 (build 17.0.9+9, mixed mode, sharing)`,
			want: 17,
		},
		{
			output: `openjdk version "21.0.1" 2023-10-17 LTS
OpenJDK Runtime Environment (build 21.0.1+12)
OpenJDK 64-Bit Server VM (build 21.0.1+12, mixed mode, sharing)`,
			want: 21,
		},
		{
			output: `java version "1.8.0_391"
Java(TM) SE Runtime Environment (build 1.8.0_391-b13)
Java HotSpot(TM) 64-Bit Server VM (build 25.391-b13, mixed mode)`,
			want: 8,
		},
	}

	for _, tc := range realOutputs {
		got, err := ParseJavaVersion(tc.output)
		if err != nil {
			t.Errorf("ParseJavaVersion real output failed: %v", err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseJavaVersion real output = %d, want %d", got, tc.want)
		}
	}
}

func TestCheckJavaVersionNotFound(t *testing.T) {
	_, err := CheckJavaVersion("/nonexistent/java")
	if err == nil {
		t.Error("expected error for nonexistent java path")
	}
}

func TestFindJavaInvalidUserPath(t *testing.T) {
	ClearJavaCache()
	path := FindJava("/nonexistent/java17")
	// Falls back to system Java if available; if empty, no Java 17+ found on system
	if path == "" {
		return
	}
	t.Logf("system Java found at: %s", path)
}
