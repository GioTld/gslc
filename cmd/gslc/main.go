package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GioTld/gslc/pkg/ast"
	"github.com/GioTld/gslc/pkg/backend/llvm"
	"github.com/GioTld/gslc/pkg/diag"
	"github.com/GioTld/gslc/pkg/ir"
	"github.com/GioTld/gslc/pkg/lexer"
	"github.com/GioTld/gslc/pkg/parser"
	"github.com/GioTld/gslc/pkg/sema"
)

const version = "0.1.0-bootstrap"

func printUsage() {
	fmt.Printf("GSL Compiler (gslc) v%s\n", version)
	fmt.Println("Usage: gslc [command] [options] <source.gsl>")
	fmt.Println("\nCommands:")
	fmt.Println("  build <file> [options]    Compile GSL source to executable binary")
	fmt.Println("  emit-llvm <file> [-o ll]  Generate textual LLVM IR")
	fmt.Println("  emit-ir <file>            Lower AST to high-level GSL IR")
	fmt.Println("  check <file>              Type-check source code and report semantic diagnostics")
	fmt.Println("  parse <file>              Parse source and display AST")
	fmt.Println("  tokenize <file>           Scan and print tokens with positions")
	fmt.Println("  version                   Display compiler version")
	fmt.Println("  help                      Show this help menu")
	fmt.Println("\nOptions:")
	fmt.Println("  -o <path>                 Specify output binary or file")
	fmt.Println("  --diagnostics=json        Emit the first diagnostic as JSON (check command)")
	fmt.Println("  -freestanding             Compile in bare-metal freestanding mode (no-std, static, no libc)")
	fmt.Println("  -T <script.ld>            Pass a linker script to the linker")
	fmt.Println("  -boot <boot.S>            Link a bootloader handoff assembly file")
	fmt.Println("  --target=<triple>         Set the LLVM target triple (e.g. x86_64-unknown-none-elf)")
	fmt.Println("  -v, --version             Display compiler version")
	fmt.Println("  -h, --help                Show this help menu")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	arg := os.Args[1]
	switch arg {
	case "-h", "--help", "help":
		printUsage()
		return
	case "-v", "--version", "version":
		fmt.Printf("gslc version %s\n", version)
		return
	case "build":
		cfg := parseBuildArgs(os.Args[2:])
		if cfg.src == "" {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'build'")
			os.Exit(1)
		}
		buildFile(cfg)
	case "emit-llvm":
		cfg := parseBuildArgs(os.Args[2:])
		if cfg.src == "" {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'emit-llvm'")
			os.Exit(1)
		}
		emitLLVMFile(cfg.src, cfg.out, cfg.target)
	case "emit-ir":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'emit-ir'")
			os.Exit(1)
		}
		emitIRFile(os.Args[2])
	case "tokenize":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'tokenize'")
			os.Exit(1)
		}
		tokenizeFile(os.Args[2])
	case "parse":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'parse'")
			os.Exit(1)
		}
		parseFile(os.Args[2])
	case "check":
		if len(os.Args) == 4 && os.Args[2] == "--diagnostics=json" {
			checkFileMode(os.Args[3], true)
			return
		}
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "error: missing source file for 'check'")
			os.Exit(1)
		}
		checkFile(os.Args[2])
	default:
		// Default to checking if a file is directly supplied
		checkFile(arg)
	}
}

func tokenizeFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to read file %q: %v\n", path, err)
		os.Exit(1)
	}

	source := string(content)
	lex := lexer.New(path, source)

	tokenCount := 0
	for {
		tok := lex.NextToken()
		fmt.Printf("%-18s %-20q [line %d, col %d]\n",
			tok.Kind, tok.Literal, tok.Span.Start.Line, tok.Span.Start.Column)
		tokenCount++

		if tok.Kind == lexer.TokenEOF {
			break
		}
	}

	diagnostics := lex.Diagnostics()
	if len(diagnostics) > 0 {
		fmt.Fprintln(os.Stderr, "\nDiagnostics encountered:")
		for _, d := range diagnostics {
			fmt.Fprint(os.Stderr, d.Format(source))
		}
		os.Exit(1)
	}

	fmt.Printf("\nTokenized %d tokens successfully.\n", tokenCount)
}

func parseFile(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to read file %q: %v\n", path, err)
		os.Exit(1)
	}

	source := string(content)
	lex := lexer.New(path, source)
	p := parser.New(lex)
	prog := p.ParseProgram()

	diagnostics := p.Diagnostics()
	if len(diagnostics) > 0 {
		fmt.Fprintln(os.Stderr, "\nDiagnostics encountered during parsing:")
		for _, d := range diagnostics {
			fmt.Fprint(os.Stderr, d.Format(source))
		}
		os.Exit(1)
	}

	fmt.Printf("Parsed %d top-level declarations successfully:\n\n", len(prog.Decls))
	fmt.Print(prog.String())
}

func checkFile(path string) {
	checkFileMode(path, false)
}

func printMachineDiagnostic(d diag.Diagnostic, fallback string) {
	json.NewEncoder(os.Stdout).Encode(d.Record(fallback))
}

func checkFileMode(path string, machine bool) {
	prog := loadProgramWithImportsMode(path, machine)

	analyzer := sema.New()
	analyzer.AnalyzeProgram(prog)

	if len(analyzer.Diagnostics()) > 0 {
		if machine {
			printMachineDiagnostic(analyzer.Diagnostics()[0], "E2000")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "\nSemantic diagnostics encountered:")
		for _, d := range analyzer.Diagnostics() {
			fmt.Fprintln(os.Stderr, d.Message)
		}
		os.Exit(1)
	}

	if !machine {
		fmt.Printf("Type check passed: %d declarations analyzed with 0 errors.\n", len(prog.Decls))
	}
}

type buildConfig struct {
	src          string
	out          string
	freestanding bool
	target       string
	linkerScript string
	bootSource   string
}

func parseBuildArgs(args []string) buildConfig {
	var cfg buildConfig
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-o" && i+1 < len(args) {
			cfg.out = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "-o=") {
			cfg.out = strings.TrimPrefix(arg, "-o=")
		} else if arg == "-freestanding" || arg == "--freestanding" {
			cfg.freestanding = true
		} else if arg == "-T" && i+1 < len(args) {
			cfg.linkerScript = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "-T=") {
			cfg.linkerScript = strings.TrimPrefix(arg, "-T=")
		} else if arg == "-boot" && i+1 < len(args) {
			cfg.bootSource = args[i+1]
		} else if (arg == "--target" || arg == "-target") && i+1 < len(args) {
			cfg.target = args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--target=") {
			cfg.target = strings.TrimPrefix(arg, "--target=")
		} else if strings.HasPrefix(arg, "-target=") {
			cfg.target = strings.TrimPrefix(arg, "-target=")
		} else if !strings.HasPrefix(arg, "-") && cfg.src == "" {
			cfg.src = arg
		}
	}
	return cfg
}

func parseArgsWithOutput(args []string) (string, string) {
	cfg := parseBuildArgs(args)
	return cfg.src, cfg.out
}

func findImportFile(currentDir, importPath string) string {
	var candidates []string

	candidates = append(candidates,
		filepath.Join(currentDir, importPath),
		filepath.Join(currentDir, importPath+".gsl"),
	)

	for _, base := range []string{currentDir, "."} {
		dir, err := filepath.Abs(base)
		if err == nil {
			for i := 0; i < 5; i++ {
				candidates = append(candidates,
					filepath.Join(dir, "lib", importPath),
					filepath.Join(dir, "lib", importPath+".gsl"),
				)
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}
	}

	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "..", "lib", importPath),
			filepath.Join(execDir, "..", "lib", importPath+".gsl"),
		)
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

func loadProgramWithImports(entryPath string) *ast.Program {
	return loadProgramWithImportsMode(entryPath, false)
}

func loadProgramWithImportsMode(entryPath string, machine bool) *ast.Program {
	visited := make(map[string]bool)
	var allDecls []ast.Decl

	var loadFile func(filePath string)
	loadFile = func(filePath string) {
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			absPath = filePath
		}
		if visited[absPath] {
			return
		}
		visited[absPath] = true

		content, err := os.ReadFile(filePath)
		if err != nil {
			if machine {
				printMachineDiagnostic(diag.NewError(diag.Span{}, "input error").WithCode("E3000"), "E3000")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "error: failed to read file %q: %v\n", filePath, err)
			os.Exit(1)
		}

		source := string(content)
		lex := lexer.New(filePath, source)
		p := parser.New(lex)
		prog := p.ParseProgram()

		if len(p.Diagnostics()) > 0 {
			if machine {
				printMachineDiagnostic(p.Diagnostics()[0], "E1000")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "\nSyntax diagnostics encountered in %s:\n", filePath)
			for _, d := range p.Diagnostics() {
				fmt.Fprint(os.Stderr, d.Format(source))
			}
			os.Exit(1)
		}

		fileDir := filepath.Dir(filePath)
		var fileDecls []ast.Decl

		for _, decl := range prog.Decls {
			if imp, ok := decl.(*ast.ImportDecl); ok {
				target := findImportFile(fileDir, imp.Path)
				if target == "" {
					if machine {
						printMachineDiagnostic(diag.NewError(diag.Span{}, "input error").WithCode("E3000"), "E3000")
						os.Exit(1)
					}
					fmt.Fprintf(os.Stderr, "error: cannot find imported module %q from %s\n", imp.Path, filePath)
					os.Exit(1)
				}
				loadFile(target)
			} else {
				fileDecls = append(fileDecls, decl)
			}
		}

		allDecls = append(allDecls, fileDecls...)
	}

	loadFile(entryPath)
	return &ast.Program{Decls: allDecls}
}

func compileToLLVM(path string) (*ast.Program, *ir.Module, string) {
	return compileToLLVMWithTarget(path, "")
}

func compileToLLVMWithTarget(path, targetTriple string) (*ast.Program, *ir.Module, string) {
	prog := loadProgramWithImports(path)

	analyzer := sema.New()
	analyzer.AnalyzeProgram(prog)

	if len(analyzer.Diagnostics()) > 0 {
		fmt.Fprintln(os.Stderr, "\nSemantic diagnostics encountered:")
		for _, d := range analyzer.Diagnostics() {
			fmt.Fprintln(os.Stderr, d.Message)
		}
		os.Exit(1)
	}

	builder := ir.NewBuilder(analyzer)
	mod := builder.Build(prog)

	gen := llvm.NewGenerator(mod)
	if targetTriple != "" {
		gen.TargetTriple = targetTriple
	}
	gen.Kernel = targetTriple == "x86_64-unknown-none-elf"
	ll := gen.Generate()

	return prog, mod, ll
}

func emitIRFile(path string) {
	_, mod, _ := compileToLLVM(path)
	fmt.Print(mod.String())
}

func emitLLVMFile(srcPath, outPath, target string) {
	_, _, ll := compileToLLVMWithTarget(srcPath, target)
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(ll), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to write %q: %v\n", outPath, err)
			os.Exit(1)
		}
		fmt.Printf("Wrote LLVM IR to %s\n", outPath)
	} else {
		fmt.Print(ll)
	}
}

func buildFile(cfg buildConfig) {
	srcPath := cfg.src
	outPath := cfg.out
	freestanding := cfg.freestanding
	target := cfg.target
	linkerScript := cfg.linkerScript
	bootSource := cfg.bootSource

	_, mod, ll := compileToLLVMWithTarget(srcPath, target)

	if outPath == "" {
		base := filepath.Base(srcPath)
		ext := filepath.Ext(base)
		outPath = strings.TrimSuffix(base, ext)
		if outPath == "" {
			outPath = "a.out"
		}
	}

	// Detect entrypoint for freestanding linking
	entryPoint := "main"
	for _, fn := range mod.Functions {
		if fn.Name == "_start" {
			entryPoint = "_start"
			break
		}
	}
	if bootSource != "" {
		entryPoint = "_start"
	}
	if freestanding && target == "x86_64-unknown-none-elf" && entryPoint != "_start" {
		fmt.Fprintln(os.Stderr, "error: kernel target requires an _start function or -boot <boot.S>")
		os.Exit(1)
	}

	if freestanding && entryPoint != "_start" {
		ll += "\ndefine void @_start() {\nentry:\n  %ret = call i32 @main()\n  %ext = zext i32 %ret to i64\n  call i64 asm sideeffect \"syscall\", \"={rax},{rax},{rdi},~{rcx},~{r11},~{memory}\"(i64 60, i64 %ext)\n  unreachable\n}\n"
		entryPoint = "_start"
	}

	llPath := outPath + ".ll"
	if err := os.WriteFile(llPath, []byte(ll), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to write intermediate LLVM IR %q: %v\n", llPath, err)
		os.Exit(1)
	}

	// 1. Try clang
	clangPath, err := exec.LookPath("clang")
	if err == nil {
		clangArgs := []string{llPath}
		if target != "" {
			clangArgs = append(clangArgs, "--target="+target)
		}
		if freestanding {
			clangArgs = append(clangArgs, "-ffreestanding", "-nostdlib", "-static", "-Wl,-e,"+entryPoint)
			if target == "x86_64-unknown-none-elf" {
				clangArgs = append(clangArgs, "-mno-red-zone", "-mgeneral-regs-only", "-mcmodel=kernel", "-fno-pic", "-no-pie")
			}
		}
		if bootSource != "" {
			clangArgs = append(clangArgs, bootSource)
		}
		if linkerScript != "" {
			clangArgs = append(clangArgs, "-T", linkerScript)
		}
		clangArgs = append(clangArgs, "-o", outPath)

		cmd := exec.Command(clangPath, clangArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: clang compilation failed: %v\n", err)
			os.Exit(1)
		}
		os.Remove(llPath)
		if freestanding {
			fmt.Printf("Successfully built freestanding binary %s (entrypoint: %s)\n", outPath, entryPoint)
		} else {
			fmt.Printf("Successfully built binary %s\n", outPath)
		}
		return
	}

	// 2. Try llc + gcc/as
	llcPath, err := exec.LookPath("llc")
	gccPath, gccErr := exec.LookPath("gcc")
	if err == nil && gccErr == nil {
		objPath := outPath + ".o"
		llcArgs := []string{"-filetype=obj"}
		if target != "" {
			llcArgs = append(llcArgs, "-mtriple="+target)
		}
		llcArgs = append(llcArgs, llPath, "-o", objPath)
		llcCmd := exec.Command(llcPath, llcArgs...)
		if err := llcCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: llc codegen failed: %v\n", err)
			os.Exit(1)
		}
		gccArgs := []string{objPath}
		if freestanding {
			gccArgs = append(gccArgs, "-nostdlib", "-static", "-Wl,-e,"+entryPoint)
		}
		if linkerScript != "" {
			gccArgs = append(gccArgs, "-T", linkerScript)
		}
		gccArgs = append(gccArgs, "-o", outPath)

		gccCmd := exec.Command(gccPath, gccArgs...)
		if err := gccCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: gcc linking failed: %v\n", err)
			os.Exit(1)
		}
		os.Remove(objPath)
		os.Remove(llPath)
		if freestanding {
			fmt.Printf("Successfully built freestanding binary %s (entrypoint: %s)\n", outPath, entryPoint)
		} else {
			fmt.Printf("Successfully built binary %s\n", outPath)
		}
		return
	}

	// 3. Fallback when native backend is not installed
	fmt.Printf("[gslc] Generated LLVM IR: %s\n", llPath)
	fmt.Printf("[gslc] Note: Native backend ('clang' or 'llc') was not found in PATH.\n")
	fmt.Printf("[gslc] To link native ELF binaries, install clang (e.g. 'sudo pacman -S clang')\n")
	fmt.Printf("[gslc] To assemble manually:\n")
	if freestanding {
		fmt.Printf("       clang %s -ffreestanding -nostdlib -static -Wl,-e,%s -o %s\n", llPath, entryPoint, outPath)
	} else {
		fmt.Printf("       clang %s -o %s\n", llPath, outPath)
	}
}
