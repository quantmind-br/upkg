# Flatpak Backend - Issues

## 2026-01-19 Implementation

### Pre-existing Issues (Not Related to Flatpak)

1. **golangci-lint v1/v2 Config Mismatch**
   - Error: "you are using a configuration file for golangci-lint v2 with golangci-lint v1"
   - Impact: `make lint` fails
   - Resolution: Update golangci-lint to v2 or downgrade config

2. **AppImage Test Failure**
   - Test: `TestAppImageBackend_extractAppImage_UnsquashfsNotFound`
   - Error: Expected "unsquashfs not found" but got actual unsquashfs error
   - Cause: Test runs on system with unsquashfs installed, error message differs
   - Resolution: Update test to handle both cases

### Blockers for Manual Testing

1. **No Flathub Remote Configured**
   - Commands like `flatpak install org.gnome.Calculator` cannot be tested
   - Suggestion shown correctly in `upkg doctor` output
   - Fix: `flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo`
