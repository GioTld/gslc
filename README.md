# GSL — Gio Systems Language

Lenguaje de sistemas con control de bajo nivel, memoria explícita y sintaxis inspirada en Go. El compilador está escrito en GSL; Python organiza las herramientas y LLVM/LLD generan y enlazan el código nativo.

**Experimental.** Este checkout contiene el compilador aceptado `0.2.0+compiler-ir`. La [release v0.2.0](https://github.com/GioTld/gslc/releases/tag/v0.2.0) es una entrega anterior; los ejemplos siguientes corresponden al checkout actual.

## Instalar y compilar

Requiere Linux x86_64, glibc 2.34+, Python 3, Clang y LLD. Entorno validado: Ubuntu 24.04 y LLVM 18.1.3. Los objetos freestanding requieren también binutils; QEMU solo se necesita para las pruebas de arranque.

Desde el repositorio, prepara e instala el compilador aceptado:

```bash
python3 scripts/package.py --output dist/gslc-linux-x86_64.tar.gz
tar -xzf dist/gslc-linux-x86_64.tar.gz
python3 gslc-linux-x86_64/install.py --prefix "$HOME/.local"
export PATH="$HOME/.local/bin:$PATH"
gslc --version
```

El instalador verifica integridad y no sobrescribe instalaciones existentes. Para probar otra versión, usa un prefijo nuevo. El paquete incluye compilador, bibliotecas y controlador; no necesita el checkout después de instalarse.

```gsl
import "std/io"

func main() u32 {
    let result = io_println("Hola mundo")
    return result.error
}
```

```bash
gslc build ejemplo.gsl --output /tmp/gsl-programa
/tmp/gsl-programa/program
```

Cada directorio de salida debe ser nuevo; conserva artefactos y `report.json`. Usa `gslc build --help` para las opciones completas.

## Freestanding y plataformas

```bash
gslc build kernel.gsl --target kernel --emit object --output /tmp/kernel-objeto
```

Produce `program.o`, sin red zone ni uso implícito de SIMD/x87. Para enlazar un ELF, omite `--emit object` y proporciona `--linker-script linker.ld`, `--entry kentry` y, si corresponde, `--boot boot.S` o `--object extra.o`. El protocolo de arranque pertenece al proyecto del kernel; los [fixtures](test/kernel/) verifican capacidades del compilador.

| Capa | Contenido |
| --- | --- |
| `core` | Memoria y strings sin dependencia de OS |
| `arch/x86_64`, `devices/pc` | Primitivas de CPU y helpers de hardware |
| `std` | Archivos, E/S y salida de procesos |
| Plataforma | Implementación de servicios del OS |

Hosted selecciona `linux-x86_64`; kernel selecciona `none`. `--platform none` rechaza imports que necesitan OS. `--platform custom --platform-root DIR` selecciona un backend externo sin fallback. Linux es incompatible con el perfil kernel. La selección y las fuentes quedan registradas en el informe.

## Biblioteca y recursos

- [std/io](lib/std/io.gsl): lectura/escritura parcial, escritura completa y mensajes a stdout/stderr.
- [std/file](lib/std/file.gsl): apertura, creación exclusiva y cierre; [std/fs](lib/std/fs.gsl) añade lectura acotada en un buffer del llamador.
- [std/types](lib/std/types.gsl): `File`, `OpenResult`, `IoResult` y errores `IO_*`, independientes de errno.
- [std/process](lib/std/process.gsl): terminación del proceso.

`File` es un recurso no copiable. Debe cerrarse explícitamente o mediante `defer`; no hay destructor automático. `fs_close` lo invalida incluso si falla. Los buffers pertenecen al llamador, no se retienen ni asignan de forma oculta. Las rutas y mensajes son strings terminados en NUL.

`IoResult` conserva cantidad, error y EOF. Las operaciones completas conservan el progreso parcial ante errores; `fs_read_into` distingue capacidad exacta de buffer insuficiente. Linux no reintenta un cierre fallido ni instala handlers de señales: un pipe cerrado puede terminar el proceso por SIGPIPE. Los [contratos ejecutables](test/bootstrap/acceptance/std/contracts.gsl) y el [ejemplo de copia](test/bootstrap/acceptance/std/copy.gsl) muestran los casos de uso.

### Implementar un backend externo

Crea estos tres módulos en `DIR`. Importa `std/types` para los tipos compartidos; no importes de vuelta los wrappers de `std/io` o `std/fs`.

| Archivo | Funciones requeridas |
| --- | --- |
| `io.gsl` | `platform_read(handle: u64, buffer: *u8, count: usize) IoResult`, `platform_write` con la misma firma, `platform_stdout() u64`, `platform_stderr() u64` |
| `fs.gsl` | `platform_open_read(path: string) OpenResult`, `platform_create_new(path: string) OpenResult`, `platform_close(handle: u64) u32` |
| `process.gsl` | `platform_exit(code: u32)`, sin retorno al llamador |

Los imports reservados `platform/io`, `platform/fs` y `platform/process` se resuelven únicamente en la plataforma seleccionada. Un módulo ausente falla; un archivo local con ese nombre no lo sustituye. El handle `u64` es un token opaco: el backend puede usarlo como índice de una tabla, sin asumir que el OS utiliza descriptores Linux.

Cada transferencia del backend devuelve hasta `count` bytes; un error devuelve cero bytes y `eof=false`. EOF se señala únicamente en lectura sin datos, con una petición no vacía. Un backend debe traducir errores desconocidos a `IO_ERROR`, normalizar apertura exclusiva y hacerse cargo de su política nativa de cierre: tras `platform_close` el token no puede volver a usarse. Linux no reintenta `close` cuando falla. Los streams estándar son prestados. Una capacidad no soportada debe devolver `IO_UNSUPPORTED`, sin simular éxito. No hay versión binaria de este contrato: los módulos se compilan juntos con `std`.

```bash
gslc build programa.gsl --target kernel --emit object \
  --platform custom --platform-root /ruta/giOS/backend \
  --output /tmp/gsl-gios-objeto
```

Esto selecciona biblioteca y produce un objeto freestanding; no implementa el arranque, ABI de procesos, syscalls ni enlace de giOS. El compilador instalado continúa siendo un programa Linux. Su salida transaccional específica de Linux está aislada en `compiler/host_linux`.

El [backend de prueba](test/bootstrap/platform) demuestra la sustitución sin syscalls Linux, y [los contratos ejecutables](test/bootstrap/acceptance/std/contracts.gsl) comprueban parciales, errores y cierres. La [copia con buffer acotado](test/bootstrap/acceptance/std/copy.gsl) separa la lógica portable de su pequeño adaptador de entrada Linux. Estas pruebas se ejecutan con el comando `test` habitual, además de los casos Linux reales y QEMU.

## Estado del lenguaje

Implementado: enteros, booleanos, strings, funciones, condicionales y bucles, structs/packed structs, arrays acotados, punteros, arenas, globals, secciones e intrínsecos x86_64. Los structs de datos infieren Copy; `resource struct` y agregados con recursos se mueven. `defer` captura argumentos al registrarse y ejecuta llamadas en orden inverso al salir del bloque.

Los préstamos son conservadores; `unsafe` mantiene responsabilidades de validez, duración y acceso a memoria. No se promete seguridad general de memoria. `Option`/`Result`, `match`, generics, interfaces y slices aún no son capacidades ejecutables de esta entrega.

El almacenamiento interno crece dinámicamente; permanecen guardas de anidamiento y agotamiento. El [manifiesto](test/bootstrap/manifest.json) mide el self-build, no fija un máximo de statements o expresiones para programas. Los errores conservan la salida anterior. Los diagnósticos semánticos con ubicación señalan el archivo original; JSON mantiene spans virtuales en bytes.

## Desarrollo y verificación

```bash
python3 scripts/gsl.py bootstrap --output /tmp/gsl-initial
python3 -m unittest discover -s test/bootstrap -p 'test_*.py' -v
python3 scripts/gsl.py test --compiler /tmp/gsl-initial/compiler \
  --clang clang-18 --linker ld.lld-18 --llvm-version 18.1.3 \
  --output /tmp/gsl-acceptance
```

La aceptación incluye positivos a `-O0`/`-O2`, rechazos y preservación de salida, nueve arranques QEMU y punto fijo de IR/ejecutable. Requiere las herramientas anteriores, QEMU y permiso de ptrace sobre procesos hijos. La validación se ejecuta localmente; GitHub Actions es manual.

La implementación está en [lib/compiler](lib/compiler/); el [inventario de aceptación](test/bootstrap/acceptance.json) define las pruebas activas. Los [contratos históricos](test/deferred/go_contracts.json) están inactivos y no demuestran soporte actual. Go no forma parte del compilador ni de su construcción.
