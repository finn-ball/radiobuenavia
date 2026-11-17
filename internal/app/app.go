package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"radiobuenavia/internal/audio"
	"radiobuenavia/internal/config"
	"radiobuenavia/internal/dropbox"
)

type App struct {
	cfg  config.Config
	jobs int
}

type downloadResult struct {
	name       string
	importPath string
	exportPath string
}

func New(cfg config.Config, jobs int) *App {
	return &App{cfg: cfg, jobs: max(1, jobs)}
}

func (a *App) Run() error {
	log.Print("Starting up...")
	jingles, err := resolveJingles(a.cfg.Paths.Jingles, a.cfg.Paths.JinglesDir)
	if err != nil {
		return err
	}

	log.Print("Checking FFmpeg audio processing...")
	if err := audio.CheckTools(); err != nil {
		return err
	}

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
	if err := a.runPass(dbx, jingles, true, liveFiles); err != nil {
		return err
	}
	if err := a.runPass(dbx, jingles, false, prerecordFiles); err != nil {
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

func (a *App) runPass(dbx *dropbox.Client, jingles []string, live bool, preproc []dropbox.FileMetadata) error {
	if len(preproc) == 0 {
		return nil
	}

	// Catch ambiguous remote destinations before concurrent work can overwrite one.
	names := make(map[string]bool, len(preproc))
	for _, file := range preproc {
		name := strings.ToLower(withMp3Ext(dbx.RenameFile(file)))
		if names[name] {
			return fmt.Errorf("multiple programmes map to the same output name: %s", name)
		}
		names[name] = true
	}
	tmpDir, err := os.MkdirTemp("", "rbv-pass-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	log.Printf("Processing with %d audio workers", min(a.jobs, len(preproc)))
	return runPipeline(context.Background(), a.jobs, preproc,
		func(ctx context.Context, file dropbox.FileMetadata) (downloadResult, error) {
			downloads := dbx.WithContext(ctx)
			dir, err := os.MkdirTemp(tmpDir, "programme-*")
			if err != nil {
				return downloadResult{}, err
			}
			result := downloadResult{
				name:       withMp3Ext(dbx.RenameFile(file)),
				importPath: filepath.Join(dir, "input.mp3"),
				exportPath: filepath.Join(dir, "output.mp3"),
			}
			log.Printf("Downloading %s", file.Name)
			err = retry(ctx, fmt.Sprintf("download %q", file.Name), 3, 2*time.Second, func() error {
				return downloads.DownloadFile(result.importPath, file.PathLower)
			})
			if err != nil {
				return result, fmt.Errorf("download %q failed: %w", file.Name, err)
			}
			return result, nil
		},
		func(ctx context.Context, result downloadResult) error {
			log.Printf("Processing %s", result.name)
			if err := audio.Process(ctx, result.importPath, result.exportPath, live, audio.GetArtist(result.name), jingles); err != nil {
				return fmt.Errorf("process %q failed: %w", result.name, err)
			}
			return nil
		},
		func(ctx context.Context, result downloadResult) error {
			uploads := dbx.WithContext(ctx)
			log.Printf("Uploading %s", result.name)
			if err := retry(ctx, fmt.Sprintf("upload %q", result.name), 3, 2*time.Second, func() error {
				return uploads.UploadFileSoundcloud(result.exportPath, result.name, a.cfg.Paths.PostprocessSoundcloud)
			}); err != nil {
				return fmt.Errorf("upload %q failed: %w", result.name, err)
			}
			log.Printf("Copying to archive %s", result.name)
			if err := retry(ctx, fmt.Sprintf("archive copy %q", result.name), 3, 2*time.Second, func() error {
				return uploads.CopyToArchive(result.name, a.cfg.Paths.PostprocessSoundcloud, a.cfg.Paths.PostprocessArchive)
			}); err != nil {
				return fmt.Errorf("archive copy %q failed: %w", result.name, err)
			}
			if err := os.RemoveAll(filepath.Dir(result.importPath)); err != nil {
				return err
			}
			log.Printf("Uploaded %q", result.name)
			return nil
		})
}

// runPipeline bounds in-flight files and joins every stage before returning.
// Downloads and uploads each remain serial, overlapping with the processing pool.
func runPipeline(ctx context.Context, jobs int, files []dropbox.FileMetadata,
	download func(context.Context, dropbox.FileMetadata) (downloadResult, error),
	process func(context.Context, downloadResult) error,
	upload func(context.Context, downloadResult) error,
) error {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	inputs := make(chan downloadResult)
	outputs := make(chan downloadResult)
	var firstErr error
	var failed sync.Once
	fail := func(err error) {
		failed.Do(func() { firstErr = err; cancel() })
	}
	var stages, processors sync.WaitGroup
	stages.Add(1)
	go func() {
		defer stages.Done()
		defer close(inputs)
		for _, file := range files {
			if ctx.Err() != nil {
				return
			}
			result, err := download(ctx, file)
			if err != nil {
				fail(err)
				return
			}
			select {
			case inputs <- result:
			case <-ctx.Done():
				return
			}
		}
	}()
	for i := 0; i < min(max(1, jobs), len(files)); i++ {
		processors.Add(1)
		go func() {
			defer processors.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case result, ok := <-inputs:
					if !ok || ctx.Err() != nil {
						return
					}
					if err := process(ctx, result); err != nil {
						fail(err)
						return
					}
					select {
					case outputs <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	stages.Add(1)
	go func() { defer stages.Done(); processors.Wait(); close(outputs) }()
	for result := range outputs {
		if ctx.Err() != nil {
			break
		}
		if err := upload(ctx, result); err != nil {
			fail(err)
			break
		}
	}
	cancel()
	stages.Wait()
	if firstErr != nil {
		return firstErr
	}
	return parent.Err()
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

func retry(ctx context.Context, operation string, attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = fn()
		if err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		if !isRetryableError(err) {
			return err
		}
		wait := retryDelay(err, delay, i)
		log.Printf("Retrying %s after error (%d/%d): %v (next attempt in %s)", operation, i+1, attempts, err, wait.Round(time.Millisecond))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func isRetryableError(err error) bool {
	var apiErr *dropbox.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func retryDelay(err error, base time.Duration, attempt int) time.Duration {
	var apiErr *dropbox.APIError
	if errors.As(err, &apiErr) {
		if wait, ok := apiErr.RetryDelay(); ok {
			return wait
		}
	}
	if base <= 0 {
		base = time.Second
	}
	backoff := base * time.Duration(1<<attempt)
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	jitterCap := base / 2
	if jitterCap < 250*time.Millisecond {
		jitterCap = 250 * time.Millisecond
	}
	jitter := time.Duration(time.Now().UnixNano() % int64(jitterCap))
	return backoff + jitter
}
