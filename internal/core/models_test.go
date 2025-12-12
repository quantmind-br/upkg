package core

import (
	"testing"
	"time"
)

func TestInstallRecord_GetDesktopFiles(t *testing.T) {
	tests := []struct {
		name     string
		record   InstallRecord
		expected []string
	}{
		{
			name: "No desktop files",
			record: InstallRecord{
				Metadata: Metadata{},
			},
			expected: nil,
		},
		{
			name: "Legacy DesktopFile",
			record: InstallRecord{
				DesktopFile: "/path/to/app.desktop",
				Metadata:    Metadata{},
			},
			expected: []string{"/path/to/app.desktop"},
		},
		{
			name: "Metadata DesktopFiles",
			record: InstallRecord{
				Metadata: Metadata{
					DesktopFiles: []string{"/path/to/app1.desktop", "/path/to/app2.desktop"},
				},
			},
			expected: []string{"/path/to/app1.desktop", "/path/to/app2.desktop"},
		},
		{
			name: "Both DesktopFile and DesktopFiles",
			record: InstallRecord{
				DesktopFile: "/path/to/legacy.desktop",
				Metadata: Metadata{
					DesktopFiles: []string{"/path/to/app1.desktop", "/path/to/app2.desktop"},
				},
			},
			expected: []string{"/path/to/app1.desktop", "/path/to/app2.desktop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.record.GetDesktopFiles()
			if len(result) != len(tt.expected) {
				t.Errorf("GetDesktopFiles() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("GetDesktopFiles()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestPackageType(t *testing.T) {
	tests := []struct {
		name     string
		pkgType  PackageType
		expected string
	}{
		{"DEB", PackageTypeDeb, "deb"},
		{"RPM", PackageTypeRpm, "rpm"},
		{"AppImage", PackageTypeAppImage, "appimage"},
		{"Tarball", PackageTypeTarball, "tarball"},
		{"Zip", PackageTypeZip, "zip"},
		{"Binary", PackageTypeBinary, "binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.pkgType) != tt.expected {
				t.Errorf("PackageType = %q, want %q", string(tt.pkgType), tt.expected)
			}
		})
	}
}

func TestInstallRecord(t *testing.T) {
	now := time.Now()
	record := InstallRecord{
		InstallID:    "test-id",
		PackageType:  PackageTypeDeb,
		Name:         "test-package",
		Version:      "1.0.0",
		InstallDate:  now,
		OriginalFile: "/path/to/package.deb",
		InstallPath:  "/opt/test-package",
		DesktopFile:  "/usr/share/applications/test.desktop",
		Metadata: Metadata{
			IconFiles:      []string{"/usr/share/icons/test.png"},
			WrapperScript:  "/opt/test-package/wrapper.sh",
			WaylandSupport: string(WaylandNative),
			InstallMethod:  InstallMethodLocal,
			ExtractedMeta: ExtractedMetadata{
				Categories:     []string{"Utility"},
				Comment:        "Test package",
				StartupWMClass: "test-package",
			},
		},
	}

	if record.InstallID != "test-id" {
		t.Errorf("InstallID = %q, want %q", record.InstallID, "test-id")
	}
	if record.PackageType != PackageTypeDeb {
		t.Errorf("PackageType = %q, want %q", record.PackageType, PackageTypeDeb)
	}
	if record.Name != "test-package" {
		t.Errorf("Name = %q, want %q", record.Name, "test-package")
	}
	if record.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", record.Version, "1.0.0")
	}
	if !record.InstallDate.Equal(now) {
		t.Errorf("InstallDate = %v, want %v", record.InstallDate, now)
	}
	if record.OriginalFile != "/path/to/package.deb" {
		t.Errorf("OriginalFile = %q, want %q", record.OriginalFile, "/path/to/package.deb")
	}
	if record.InstallPath != "/opt/test-package" {
		t.Errorf("InstallPath = %q, want %q", record.InstallPath, "/opt/test-package")
	}
	if record.DesktopFile != "/usr/share/applications/test.desktop" {
		t.Errorf("DesktopFile = %q, want %q", record.DesktopFile, "/usr/share/applications/test.desktop")
	}
}

func TestMetadata(t *testing.T) {
	metadata := Metadata{
		IconFiles:      []string{"/usr/share/icons/test.png"},
		WrapperScript:  "/opt/test-package/wrapper.sh",
		WaylandSupport: string(WaylandNative),
		InstallMethod:  InstallMethodLocal,
		ExtractedMeta: ExtractedMetadata{
			Categories:     []string{"Utility"},
			Comment:        "Test package",
			StartupWMClass: "test-package",
		},
	}

	if len(metadata.IconFiles) != 1 {
		t.Errorf("IconFiles length = %d, want %d", len(metadata.IconFiles), 1)
	}
	if metadata.WrapperScript != "/opt/test-package/wrapper.sh" {
		t.Errorf("WrapperScript = %q, want %q", metadata.WrapperScript, "/opt/test-package/wrapper.sh")
	}
	if metadata.WaylandSupport != string(WaylandNative) {
		t.Errorf("WaylandSupport = %q, want %q", metadata.WaylandSupport, string(WaylandNative))
	}
	if metadata.InstallMethod != InstallMethodLocal {
		t.Errorf("InstallMethod = %q, want %q", metadata.InstallMethod, InstallMethodLocal)
	}
}

func TestExtractedMetadata(t *testing.T) {
	extractedMeta := ExtractedMetadata{
		Categories:     []string{"Utility"},
		Comment:        "Test package",
		StartupWMClass: "test-package",
	}

	if len(extractedMeta.Categories) != 1 {
		t.Errorf("Categories length = %d, want %d", len(extractedMeta.Categories), 1)
	}
	if extractedMeta.Comment != "Test package" {
		t.Errorf("Comment = %q, want %q", extractedMeta.Comment, "Test package")
	}
	if extractedMeta.StartupWMClass != "test-package" {
		t.Errorf("StartupWMClass = %q, want %q", extractedMeta.StartupWMClass, "test-package")
	}
}

func TestDesktopEntry(t *testing.T) {
	entry := DesktopEntry{
		Type:           "Application",
		Version:        "1.0",
		Name:           "Test App",
		GenericName:    "Test",
		Comment:        "Test application",
		Icon:           "test-icon",
		Exec:           "/usr/bin/test",
		Path:           "/usr/bin",
		Terminal:       false,
		Categories:     []string{"Utility"},
		MimeType:       []string{"text/plain"},
		StartupWMClass: "test-app",
		NoDisplay:      false,
		Keywords:       []string{"test"},
		StartupNotify:  true,
	}

	if entry.Type != "Application" {
		t.Errorf("Type = %q, want %q", entry.Type, "Application")
	}
	if entry.Version != "1.0" {
		t.Errorf("Version = %q, want %q", entry.Version, "1.0")
	}
	if entry.Name != "Test App" {
		t.Errorf("Name = %q, want %q", entry.Name, "Test App")
	}
	if entry.GenericName != "Test" {
		t.Errorf("GenericName = %q, want %q", entry.GenericName, "Test")
	}
	if entry.Comment != "Test application" {
		t.Errorf("Comment = %q, want %q", entry.Comment, "Test application")
	}
	if entry.Icon != "test-icon" {
		t.Errorf("Icon = %q, want %q", entry.Icon, "test-icon")
	}
	if entry.Exec != "/usr/bin/test" {
		t.Errorf("Exec = %q, want %q", entry.Exec, "/usr/bin/test")
	}
	if entry.Path != "/usr/bin" {
		t.Errorf("Path = %q, want %q", entry.Path, "/usr/bin")
	}
	if entry.Terminal {
		t.Errorf("Terminal = %v, want false", entry.Terminal)
	}
	if len(entry.Categories) != 1 || entry.Categories[0] != "Utility" {
		t.Errorf("Categories = %v, want [Utility]", entry.Categories)
	}
	if len(entry.MimeType) != 1 || entry.MimeType[0] != "text/plain" {
		t.Errorf("MimeType = %v, want [text/plain]", entry.MimeType)
	}
	if entry.StartupWMClass != "test-app" {
		t.Errorf("StartupWMClass = %q, want %q", entry.StartupWMClass, "test-app")
	}
	if entry.NoDisplay {
		t.Errorf("NoDisplay = %v, want false", entry.NoDisplay)
	}
	if len(entry.Keywords) != 1 || entry.Keywords[0] != "test" {
		t.Errorf("Keywords = %v, want [test]", entry.Keywords)
	}
	if !entry.StartupNotify {
		t.Errorf("StartupNotify = %v, want true", entry.StartupNotify)
	}
}

func TestIconFile(t *testing.T) {
	icon := IconFile{
		Path: "/usr/share/icons/test.png",
		Size: "256x256",
		Ext:  "png",
	}

	if icon.Path != "/usr/share/icons/test.png" {
		t.Errorf("Path = %q, want %q", icon.Path, "/usr/share/icons/test.png")
	}
	if icon.Size != "256x256" {
		t.Errorf("Size = %q, want %q", icon.Size, "256x256")
	}
	if icon.Ext != "png" {
		t.Errorf("Ext = %q, want %q", icon.Ext, "png")
	}
}

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitGeneral", ExitGeneral, 1},
		{"ExitInvalidArgs", ExitInvalidArgs, 2},
		{"ExitInstallFailed", ExitInstallFailed, 3},
		{"ExitUninstallFailed", ExitUninstallFailed, 4},
		{"ExitDatabase", ExitDatabase, 5},
		{"ExitPermission", ExitPermission, 6},
		{"ExitNetwork", ExitNetwork, 7},
		{"ExitCommandNotFound", ExitCommandNotFound, 8},
		{"ExitInterrupted", ExitInterrupted, 130},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.expected)
			}
		})
	}
}

func TestWaylandSupport(t *testing.T) {
	tests := []struct {
		name     string
		support  WaylandSupport
		expected string
	}{
		{"Unknown", WaylandUnknown, "unknown"},
		{"Native", WaylandNative, "native"},
		{"XWayland", WaylandXWayland, "xwayland"},
		{"Hybrid", WaylandHybrid, "hybrid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.support) != tt.expected {
				t.Errorf("WaylandSupport = %q, want %q", string(tt.support), tt.expected)
			}
		})
	}
}

func TestInstallOptions(t *testing.T) {
	options := InstallOptions{
		Force:          true,
		SkipDesktop:    true,
		CustomName:     "custom-name",
		SkipWaylandEnv: true,
	}

	if !options.Force {
		t.Errorf("Force = %v, want %v", options.Force, true)
	}
	if !options.SkipDesktop {
		t.Errorf("SkipDesktop = %v, want %v", options.SkipDesktop, true)
	}
	if options.CustomName != "custom-name" {
		t.Errorf("CustomName = %q, want %q", options.CustomName, "custom-name")
	}
	if !options.SkipWaylandEnv {
		t.Errorf("SkipWaylandEnv = %v, want %v", options.SkipWaylandEnv, true)
	}
}
