package echolocal

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/layout"
	"io"
	"os"
	"strings"
)

func avatarRead(path string, limit int) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if e != nil {
		return nil, e
	}
	if len(b) > limit {
		return nil, fmt.Errorf("asset exceeds %d bytes", limit)
	}
	return b, nil
}
func newAvatarCmd() *cobra.Command {
	var serial, modelPath, motionPath string
	cmd := &cobra.Command{Use: "avatar", Short: "Manage local avatar assets"}
	install := &cobra.Command{Use: "import", Short: "Validate and atomically select a local VRM and optional VRMA", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		model, err := avatarRead(modelPath, avatar.MaxModelBytes)
		if err != nil {
			return err
		}
		var motion []byte
		if motionPath != "" {
			motion, err = avatarRead(motionPath, avatar.MaxMotionBytes)
			if err != nil {
				return err
			}
		}
		a, err := avatar.PrepareAssets(model, motion)
		if err != nil {
			return err
		}
		d, err := connect(c.Context(), c.OutOrStdout(), serial)
		if err != nil {
			return err
		}
		err = installAvatarAssets(d, a, model, motion)
		if err == nil {
			fmt.Fprintln(c.OutOrStdout(), "Avatar assets selected. Stop and start Avatar Lab to use them.")
		}
		return err
	}}
	install.Flags().StringVar(&serial, "serial", "", "device to act on")
	install.Flags().StringVar(&modelPath, "model", "", "local self-contained VRM file")
	install.Flags().StringVar(&motionPath, "animation", "", "optional local self-contained VRMA file")
	_ = install.MarkFlagRequired("model")
	cmd.AddCommand(install)
	return cmd
}

type avatarDevice interface {
	Getprop(string) (string, error)
	Shell(string) (string, error)
	WriteFile(string, []byte, os.FileMode) error
}

func installAvatarAssets(d avatarDevice, a avatar.AssetSet, model, motion []byte) error {
	device, err := d.Getprop("ro.product.device")
	if err != nil {
		return err
	}
	if device != "athena" {
		return fmt.Errorf("native avatar rendering is supported on Athena only")
	}
	if _, err = d.Shell("mkdir -p " + layout.AvatarDir + " && chmod 700 " + layout.AvatarDir); err != nil {
		return err
	}
	write := func(hash, suffix string, b []byte) error {
		p := layout.AvatarDir + "/" + hash + suffix
		if err := d.WriteFile(p, b, 0o600); err != nil {
			return err
		}
		out, err := d.Shell("sha256sum " + p)
		if err != nil {
			return err
		}
		f := strings.Fields(out)
		if len(f) < 1 || f[0] != hash {
			return fmt.Errorf("transferred asset checksum mismatch")
		}
		return nil
	}
	if err = write(a.Model, ".vrm", model); err != nil {
		return err
	}
	if a.Motion != "" {
		if err = write(a.Motion, ".vrma", motion); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err = d.WriteFile(layout.AvatarDir+"/current.new.json", raw, 0o600); err != nil {
		return err
	}
	_, err = d.Shell("cd " + layout.AvatarDir + " && if [ -f current.json ]; then cp current.json previous.json || exit 1; fi; mv current.new.json current.json")
	return err
}
