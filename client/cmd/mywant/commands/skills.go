package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const defaultSkillsRepo = "https://github.com/onelittlenightmusic/mywant-skills.git"

var (
	skillsSource string
	skillsForce  bool
)

var SkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Install MyWant skills for agents",
	Long: `Install MyWant skills into ~/.mywant/skills, then link them into agent-specific skill directories.

Examples:
  mywant skills install
  mywant skills install gemini
  mywant skills install claude
  mywant skills install codex
  mywant skills list
  mywant skills list gemini
  mywant skills uninstall gemini
  mywant skills uninstall`,
}

var skillsListCmd = &cobra.Command{
	Use:   "list [gemini|claude|codex]",
	Short: "List installed MyWant skills and agent link status",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		target := ""
		if len(args) > 0 {
			target = args[0]
			if _, err := agentSkillsDir(target); err != nil {
				fmt.Fprintf(os.Stderr, "Error listing skills: %v\n", err)
				os.Exit(1)
			}
		}
		if err := listSkills(myWantSkillsPath(), target); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing skills: %v\n", err)
			os.Exit(1)
		}
	},
}

var skillsInstallCmd = &cobra.Command{
	Use:   "install [gemini|claude|codex]",
	Short: "Install MyWant skills and optionally link them for an agent",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repoPath, err := installMyWantSkills(skillsSource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error installing skills: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Installed skills to %s\n", repoPath)

		if len(args) == 0 {
			return
		}
		target := args[0]
		if err := linkSkillsForAgent(repoPath, target, skillsForce); err != nil {
			fmt.Fprintf(os.Stderr, "Error linking skills for %s: %v\n", target, err)
			os.Exit(1)
		}
		fmt.Printf("Linked skills for %s\n", target)
	},
}

var skillsUninstallCmd = &cobra.Command{
	Use:   "uninstall [gemini|claude|codex]",
	Short: "Uninstall MyWant skills or unlink them from an agent",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repoPath := myWantSkillsPath()
		if len(args) > 0 {
			target := args[0]
			if err := unlinkSkillsForAgent(repoPath, target, skillsForce); err != nil {
				fmt.Fprintf(os.Stderr, "Error unlinking skills for %s: %v\n", target, err)
				os.Exit(1)
			}
			fmt.Printf("Unlinked skills for %s\n", target)
			return
		}

		if err := uninstallMyWantSkills(repoPath, skillsForce); err != nil {
			fmt.Fprintf(os.Stderr, "Error uninstalling skills: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed skills from %s\n", repoPath)
	},
}

func listSkills(repoPath, target string) error {
	names, err := skillNames(repoPath)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("No MyWant skills installed at %s\n", repoPath)
		return nil
	}

	targets := []string{}
	if target != "" {
		targets = []string{target}
	} else {
		targets = []string{"gemini", "claude", "codex"}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprint(w, "SKILL\tPATH")
	for _, t := range targets {
		fmt.Fprintf(w, "\t%s", strings.ToUpper(t))
	}
	fmt.Fprintln(w)
	for _, name := range names {
		src := filepath.Join(repoPath, name)
		fmt.Fprintf(w, "%s\t%s", name, src)
		for _, t := range targets {
			status, err := agentSkillStatus(src, name, t)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "\t%s", status)
		}
		fmt.Fprintln(w)
	}
	return w.Flush()
}

func agentSkillStatus(src, name, target string) (string, error) {
	dstBase, err := agentSkillsDir(target)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dstBase, name)
	link, err := os.Readlink(dst)
	if err == nil {
		if link == src {
			return "linked", nil
		}
		return "linked-other", nil
	}
	if os.IsNotExist(err) {
		return "-", nil
	}
	if info, statErr := os.Lstat(dst); statErr == nil && info.Mode()&os.ModeSymlink == 0 {
		return "exists", nil
	}
	return "", err
}

func myWantSkillsPath() string {
	return filepath.Join(getMyWantDir(), "skills", "mywant-skills")
}

func installMyWantSkills(source string) (string, error) {
	if source == "" {
		source = defaultSkillsRepo
	}

	dst := myWantSkillsPath()
	if isLocalDir(source) {
		if err := replaceDirFromLocal(source, dst); err != nil {
			return "", err
		}
		return dst, nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required to install from %s", source)
	}

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return "", err
		}
		return dst, runGit("", "clone", source, dst)
	} else if err != nil {
		return "", err
	}

	if !isGitRepo(dst) {
		return "", fmt.Errorf("%s already exists and is not a git repository; move it away or use --source with a local directory", dst)
	}
	if err := runGit(dst, "pull", "--ff-only"); err != nil {
		return "", err
	}
	return dst, nil
}

func linkSkillsForAgent(repoPath, target string, force bool) error {
	dstBase, err := agentSkillsDir(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstBase, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return err
	}
	linked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(repoPath, entry.Name())
		if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
			continue
		}
		dst := filepath.Join(dstBase, entry.Name())
		if err := replaceSymlink(src, dst, force); err != nil {
			return err
		}
		linked++
	}
	if linked == 0 {
		return fmt.Errorf("no skills with SKILL.md found in %s", repoPath)
	}
	return nil
}

func unlinkSkillsForAgent(repoPath, target string, force bool) error {
	dstBase, err := agentSkillsDir(target)
	if err != nil {
		return err
	}

	names, err := skillNames(repoPath)
	if err != nil {
		return err
	}
	for _, name := range names {
		dst := filepath.Join(dstBase, name)
		if err := removeInstalledSkillLink(dst, force); err != nil {
			return err
		}
	}
	return nil
}

func uninstallMyWantSkills(repoPath string, force bool) error {
	info, err := os.Lstat(repoPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(repoPath)
	}
	if !info.IsDir() {
		if !force {
			return fmt.Errorf("%s exists and is not a directory; use --force to remove it", repoPath)
		}
		return os.Remove(repoPath)
	}
	if !force && !isGitRepo(repoPath) {
		return fmt.Errorf("%s is not a git repository; use --force to remove it", repoPath)
	}
	return os.RemoveAll(repoPath)
}

func skillNames(repoPath string) ([]string, error) {
	entries, err := os.ReadDir(repoPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoPath, entry.Name(), "SKILL.md")); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func removeInstalledSkillLink(path string, force bool) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 && !force {
		return fmt.Errorf("%s exists and is not a symlink; use --force to remove it", path)
	}
	return os.RemoveAll(path)
}

func agentSkillsDir(target string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch target {
	case "gemini":
		return filepath.Join(home, ".gemini", "skills"), nil
	case "claude":
		return filepath.Join(home, ".claude", "skills"), nil
	case "codex":
		return filepath.Join(home, ".codex", "skills"), nil
	default:
		return "", fmt.Errorf("unknown target %q (expected gemini, claude, or codex)", target)
	}
}

func replaceSymlink(src, dst string, force bool) error {
	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink == 0 && !force {
			return fmt.Errorf("%s already exists and is not a symlink; use --force to replace it", dst)
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(src, dst)
}

func isLocalDir(path string) bool {
	if strings.Contains(path, "://") || strings.HasSuffix(path, ".git") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func replaceDirFromLocal(src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if srcAbs == dstAbs {
		return nil
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, out, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func init() {
	skillsInstallCmd.Flags().StringVar(&skillsSource, "source", "", "local directory or git URL for mywant-skills")
	skillsInstallCmd.Flags().BoolVar(&skillsForce, "force", false, "replace existing non-symlink files in the target agent skills directory")
	skillsUninstallCmd.Flags().BoolVar(&skillsForce, "force", false, "remove non-symlink files or non-git skill directories")
	SkillsCmd.AddCommand(skillsListCmd)
	SkillsCmd.AddCommand(skillsInstallCmd)
	SkillsCmd.AddCommand(skillsUninstallCmd)
}
