package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ImportDFLOPHumanPack derives technical metadata; identity and rights remain
// explicit operator attestations. It performs no identity inference or upload.
func ImportDFLOPHumanPack(directory string) ([]VerificationMediaFixture, error) {
	metadata, err := os.ReadFile(filepath.Join(directory, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var rights struct {
		IdentityClassification string `json:"identity_classification"`
		RightsBasis            string `json:"rights_basis"`
		GenerationProvenance   string `json:"generation_provenance"`
		ConsentReference       string `json:"consent_reference"`
	}
	if common.Unmarshal(metadata, &rights) != nil || rights.RightsBasis == "" {
		return nil, errors.New("FIXTURE_AUTHORIZATION_REQUIRED")
	}
	synthetic := rights.IdentityClassification == "SYNTHETIC_FICTIONAL_PERSON"
	if synthetic && rights.GenerationProvenance == "" || !synthetic && (rights.IdentityClassification != "CONSENTED_TEST_PERFORMER" || rights.ConsentReference == "") {
		return nil, errors.New("FIXTURE_AUTHORIZATION_REQUIRED")
	}
	result := []VerificationMediaFixture{}
	for _, asset := range []struct{ id, name, kind, mime string }{{"human-portrait-v1", "portrait.png", "portrait", "image/png"}, {"human-face-video-v1", "face-video.mp4", "face_video", "video/mp4"}, {"human-motion-video-v1", "motion-video.mp4", "motion_video", "video/mp4"}} {
		data, err := verificationHumanAssetBytes(directory, asset.name)
		if err != nil {
			return nil, err
		}
		fixture := VerificationMediaFixture{ID: asset.id, Path: "human-pack/" + asset.name, AssetType: asset.kind, MIMEType: asset.mime, Bytes: len(data), SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), RequiresFace: true, Synthetic: synthetic, IdentityClassification: rights.IdentityClassification, RightsBasis: rights.RightsBasis, CreatedAt: time.Now().UTC().Format(time.RFC3339), ConsentReference: rights.ConsentReference, Provenance: rights.GenerationProvenance, RightsClassification: "fictional-human-test-only"}
		if !synthetic {
			fixture.RightsClassification = "organization-performer-explicit-consent"
			fixture.Provenance = rights.ConsentReference
		}
		if asset.kind == "portrait" {
			image, err := png.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				return nil, errors.New("FIXTURE_INVALID_IMAGE")
			}
			fixture.Width, fixture.Height = image.Width, image.Height
		} else {
			atoms, err := dflopVideoAtoms(data)
			if err != nil || len(atoms["mvhd"]) == 0 || len(atoms["tkhd"]) == 0 {
				return nil, errors.New("FIXTURE_INVALID_VIDEO")
			}
			movie, track := atoms["mvhd"][0], atoms["tkhd"][0]
			if len(movie) < 20 || movie[0] != 0 || len(track) < 8 {
				return nil, errors.New("FIXTURE_INVALID_VIDEO")
			}
			scale := binary.BigEndian.Uint32(movie[12:16])
			if scale == 0 {
				return nil, errors.New("FIXTURE_INVALID_VIDEO")
			}
			fixture.Seconds = float64(binary.BigEndian.Uint32(movie[16:20])) / float64(scale)
			fixture.Width = int(binary.BigEndian.Uint32(track[len(track)-8:len(track)-4]) >> 16)
			fixture.Height = int(binary.BigEndian.Uint32(track[len(track)-4:]) >> 16)
		}
		if err := validateDFLOPVerificationMediaBytes(fixture, data); err != nil {
			return nil, err
		}
		result = append(result, fixture)
	}
	return result, nil
}

func verificationHumanAssetBytes(directory, name string) ([]byte, error) {
	if name != "portrait.png" && name != "face-video.mp4" && name != "motion-video.mp4" {
		return nil, errors.New("FIXTURE_ASSET_NOT_ALLOWLISTED")
	}
	if filepath.Base(name) != name {
		return nil, errors.New("FIXTURE_ASSET_NOT_ALLOWLISTED")
	}
	info, err := os.Lstat(filepath.Join(directory, name))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 16<<20 {
		return nil, errors.New("FIXTURE_ASSET_NOT_ALLOWLISTED")
	}
	return os.ReadFile(filepath.Join(directory, name))
}

// Only an explicit operator registry can extend the immutable public allowlist.
// Changed bytes invalidate the registered SHA; no URL maps to an arbitrary path.
func verificationHumanRegistry() []VerificationMediaFixture {
	directory := os.Getenv("VERIFICATION_HUMAN_ASSET_DIRECTORY")
	if directory == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(directory, "registry.json"))
	if err != nil {
		return nil
	}
	var fixtures []VerificationMediaFixture
	if common.Unmarshal(data, &fixtures) != nil || len(fixtures) != 3 {
		return nil
	}
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		expected := map[string]string{"human-portrait-v1": "human-pack/portrait.png", "human-face-video-v1": "human-pack/face-video.mp4", "human-motion-video-v1": "human-pack/motion-video.mp4"}
		if expected[fixture.ID] != fixture.Path || seen[fixture.ID] {
			return nil
		}
		seen[fixture.ID] = true
		if !fixture.RequiresFace || !strings.HasPrefix(fixture.Path, "human-pack/") {
			return nil
		}
		name := strings.TrimPrefix(fixture.Path, "human-pack/")
		data, err := verificationHumanAssetBytes(directory, name)
		if err != nil || validateDFLOPVerificationMediaBytes(fixture, data) != nil {
			return nil
		}
	}
	return fixtures
}

func verificationMediaBytes(fixture VerificationMediaFixture) ([]byte, error) {
	if !strings.HasPrefix(fixture.Path, "human-pack/") {
		return dflopVerificationFiles.ReadFile(fixture.Path)
	}
	for _, registered := range verificationHumanRegistry() {
		if registered.ID == fixture.ID && registered.Path == fixture.Path && registered.SHA256 == fixture.SHA256 {
			return verificationHumanAssetBytes(os.Getenv("VERIFICATION_HUMAN_ASSET_DIRECTORY"), strings.TrimPrefix(fixture.Path, "human-pack/"))
		}
	}
	return nil, errors.New("FIXTURE_ASSET_NOT_ALLOWLISTED")
}
