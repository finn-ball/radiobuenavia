package app

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"radiobuenavia/internal/audacity"
	"radiobuenavia/internal/audio"
	"radiobuenavia/internal/config"
	"radiobuenavia/internal/dropbox"
)

type App struct {
	cfg config.Config
}

type downloadResult struct {
	file       dropbox.FileMetadata
	name       string
	importPath string
	exportPath string
	err        error
}

func New(cfg config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	log.Print("Starting up...")
	jingles, err := resolveJingles(a.cfg.Paths.Jingles, a.cfg.Paths.JinglesDir)
	if err != nil {
		return err
	}

	log.Print("Connecting to Audacity...")
	pipe, err := audacity.NewPipeClient()
	if err != nil {
		return err
	}
	defer pipe.Close()

	log.Print("Connecting to Dropbox...")
	dbx, err := dropbox.NewClient(a.cfg.Auth.AppKey, a.cfg.Auth.AppSecret, a.cfg.Auth.RefreshToken)
	if err != nil {
		return err
	}

	log.Print("Listing preprocess folders...")
	liveFiles, err := a.listPass(dbx, "live", a.cfg.Paths.PreprocessLive)
	if err != nil {
		return err
	}
	prerecordFiles, err := a.listPass(dbx, "prerecord", a.cfg.Paths.PreprocessPrerecord)
	if err != nil {
		return err
	}
	if len(liveFiles) == 0 && len(prerecordFiles) == 0 {
		return nil
	}
	if !promptConfirm("\nProceed? (Y/n)") {
		log.Print("Goodbye!")
		return nil
	}
	if err := a.runPass(dbx, pipe, a.cfg.Paths.PreprocessLive, jingles, true, liveFiles); err != nil {
		return err
	}
	if err := a.runPass(dbx, pipe, a.cfg.Paths.PreprocessPrerecord, jingles, false, prerecordFiles); err != nil {
		return err
	}
	return nil
}

func (a *App) listPass(dbx *dropbox.Client, label, preprocessPath string) ([]dropbox.FileMetadata, error) {
	if strings.TrimSpace(preprocessPath) == "" {
		return nil, nil
	}
	preproc, err := dbx.ListFilesToProcess(preprocessPath, a.cfg.Paths.PostprocessArchive)
	if err != nil {
		return nil, err
	}
	preproc = filterMp3Files(preproc)
	if len(preproc) == 0 {
		log.Printf("No new files to process in %s.", preprocessPath)
		return nil, nil
	}
	fmt.Printf("\nFiles to process (%s) (%d):\n\n", label, len(preproc))
	for _, file := range preproc {
		fmt.Printf("%s -> %s\n", file.Name, withMp3Ext(dbx.RenameFile(file)))
	}
	return preproc, nil
}

func (a *App) runPass(dbx *dropbox.Client, pipe *audacity.PipeClient, preprocessPath string, jingles []string, live bool, preproc []dropbox.FileMetadata) error {
	if len(preproc) == 0 {
		return nil
	}

	tmpDir := filepath.Join(os.TempDir(), "rbv")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}

	uploadCh, uploadDone := a.startUploadWorker(dbx)
	results := a.startDownloadWorker(dbx, preproc, tmpDir)

	for i := 0; i < len(preproc); i++ {
		result, ok := <-results
		if !ok {
			return fmt.Errorf("download worker stopped unexpectedly")
		}
		if result.err != nil {
			return result.err
		}

		fmt.Printf("\n\tProcessing:\n\t\t%s\n\n", result.name)

		if err := processFile(pipe, result, live, jingles); err != nil {
			return err
		}

		uploadCh <- uploadTask{
			name: result.name,
			path: result.exportPath,
		}
	}

	close(uploadCh)
	if err := <-uploadDone; err != nil {
		return err
	}
	return nil
}

func (a *App) startDownloadWorker(dbx *dropbox.Client, preproc []dropbox.FileMetadata, tmpDir string) <-chan downloadResult {
	results := make(chan downloadResult, 1)
	go func() {
		defer close(results)
		for _, file := range preproc {
			name := dbx.RenameFile(file)
			exportName := withMp3Ext(name)
			cleanName := strings.ReplaceAll(name, " ", "-")
			cleanExportName := strings.ReplaceAll(exportName, " ", "-")
			importPath := filepath.Join(tmpDir, "im-"+cleanName)
			exportPath := filepath.Join(tmpDir, "ex-"+cleanExportName)

			log.Printf("Downloading %s to %s", file.Name, importPath)
			if err := retry(3, 2*time.Second, func() error {
				return dbx.DownloadFile(importPath, file.PathLower)
			}); err != nil {
				results <- downloadResult{err: err}
				return
			}
			results <- downloadResult{
				file:       file,
				name:       exportName,
				importPath: importPath,
				exportPath: exportPath,
			}
		}
	}()
	return results
}

func processFile(pipe *audacity.PipeClient, result downloadResult, live bool, jingles []string) error {
	log.Print("Processing...")
	if err := pipe.Process(result.importPath, result.exportPath, live); err != nil {
		return err
	}
	log.Print("Done!")

	artist := audio.GetArtist(result.name)
	log.Print("Setting artist name and potentially changing bitrate.")
	if err := audio.ProcessMetadataAndBitrate(result.exportPath, artist, jingles); err != nil {
		return err
	}
	return nil
}

type uploadTask struct {
	name string
	path string
}

func (a *App) startUploadWorker(dbx *dropbox.Client) (chan<- uploadTask, <-chan error) {
	tasks := make(chan uploadTask, 1)
	done := make(chan error, 1)
	go func() {
		var firstErr error
		for task := range tasks {
			if firstErr != nil {
				continue
			}
			log.Printf("Uploading... %s", task.name)
			if err := retry(3, 2*time.Second, func() error {
				return dbx.UploadFileSoundcloud(task.path, task.name, a.cfg.Paths.PostprocessSoundcloud)
			}); err != nil {
				firstErr = err
				continue
			}
			log.Printf("Copying to archive... %s", task.name)
			if err := retry(3, 2*time.Second, func() error {
				return dbx.CopyToArchive(task.name, a.cfg.Paths.PostprocessSoundcloud, a.cfg.Paths.PostprocessArchive)
			}); err != nil {
				firstErr = err
				continue
			}
			log.Printf("Uploaded %q", task.name)
		}
		done <- firstErr
	}()
	return tasks, done
}

func validateJingles(jingles []string) error {
	for _, path := range jingles {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("could not find jingle: %s", path)
		}
	}
	return nil
}

func resolveJingles(jingles []string, jinglesDir string) ([]string, error) {
	if err := validateJingles(jingles); err != nil {
		return nil, err
	}
	if jinglesDir == "" {
		return jingles, nil
	}
	info, err := os.Stat(jinglesDir)
	if err != nil {
		return nil, fmt.Errorf("could not find jingles_dir: %s", jinglesDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("jingles_dir is not a directory: %s", jinglesDir)
	}
	found, err := findMp3s(jinglesDir)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		log.Printf("No mp3 files found in jingles_dir: %s", jinglesDir)
	}
	return append(jingles, found...), nil
}

func findMp3s(root string) ([]string, error) {
	var out []string
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".mp3" {
			out = append(out, filepath.Join(root, entry.Name()))
		}
	}
	return out, nil
}

func withMp3Ext(name string) string {
	ext := filepath.Ext(name)
	if strings.EqualFold(ext, ".mp3") {
		return name
	}
	return strings.TrimSuffix(name, ext) + ".mp3"
}

func filterMp3Files(files []dropbox.FileMetadata) []dropbox.FileMetadata {
	if len(files) == 0 {
		return files
	}
	out := make([]dropbox.FileMetadata, 0, len(files))
	for _, file := range files {
		if strings.EqualFold(filepath.Ext(file.Name), ".mp3") {
			out = append(out, file)
		}
	}
	return out
}

func promptConfirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	return line == "" || strings.EqualFold(line, "y")
}

func retry(attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			log.Printf("Retrying after error (%d/%d)...", i+1, attempts)
			time.Sleep(delay)
		}
		err = fn()
		if err == nil {
			return nil
		}
	}
	return err
}
