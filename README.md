# GSL — Gio Systems Language

Compilador de un lenguaje de sistemas para Linux x86_64 y entornos freestanding x86_64. Está escrito en GSL y puede compilarse a sí mismo. Python organiza la compilación y LLVM genera el código nativo.

## Instalar y empezar

La distribución inicial requiere Linux x86_64 con glibc 2.34 o posterior, Python 3, Clang y LLD. Los objetos freestanding también requieren binutils. El entorno verificado es Ubuntu 24.04 con Clang/LLD 18.1.3; otras distros y musl todavía no están validadas. No requiere Go ni QEMU para compilar programas.

**0.2.0 es experimental** y cambia las API e imports de biblioteca respecto de 0.1.0. Descarga `gslc-linux-x86_64.tar.gz` y `SHA256SUMS` desde la [release 0.2.0](https://github.com/GioTld/gslc/releases/tag/v0.2.0), y verifica el archivo con `sha256sum -c SHA256SUMS`. Los ejemplos siguientes corresponden a esta versión.

También puedes preparar el paquete desde el repositorio:

```bash
python3 scripts/package.py --output dist/gslc-linux-x86_64.tar.gz
```

El archivo incluye el compilador aceptado, las bibliotecas y el controlador de compilación. Se puede copiar a otra máquina compatible e instalar sin clonar el repositorio ni usar privilegios:

```bash
tar -xzf gslc-linux-x86_64.tar.gz
python3 gslc-linux-x86_64/install.py --prefix "$HOME/.local"
export PATH="$HOME/.local/bin:$PATH"
gslc --version
```

El instalador comprueba los archivos y no sobrescribe instalaciones existentes. Para actualizar, usa otro prefijo o elimina antes únicamente `PREFIX/bin/gslc` y `PREFIX/lib/gslc`. No modifica la configuración del shell.

Crea `ejemplo.gsl`:

```gsl
import "std/io"

func main() u32 {
    io_println("Hola mundo")
    return 0
}
```

Compila y ejecuta:

```bash
gslc build ejemplo.gsl --output /tmp/gsl-programa
/tmp/gsl-programa/program
```

Los directorios de salida deben ser nuevos. Cada comando conserva sus artefactos y un `report.json`.

## Código freestanding

Para generar un objeto que puedas enlazar con tu kernel:

```bash
gslc build /ruta/kernel.gsl --target kernel --emit object \
  --output /tmp/kernel-objeto
```

El resultado es `program.o`. Para generar un ELF con tu arranque y linker script:

```bash
gslc build /ruta/kernel.gsl --target kernel \
  --boot /ruta/boot.S --linker-script /ruta/linker.ld \
  --entry kentry --output /tmp/kernel-elf
```

`--boot` es opcional y `--object /ruta/extra.o` permite añadir objetos al enlace. El perfil freestanding deshabilita red zone, SIMD y x87; incluye helpers de memoria y rechaza syscalls de Linux. Los [fixtures de kernel](test/kernel/) muestran memoria, secciones, puertos, descriptores y paginación. Son pruebas del compilador; el proyecto del kernel aporta su protocolo de arranque.

## Biblioteca portable y plataformas

`core/string` y `core/mem` no necesitan un sistema operativo. Las primitivas x86_64 están en `arch/x86_64/cpu` y `arch/x86_64/descriptors`; los helpers PC, en `devices/pc/serial` y `devices/pc/vga`. Los módulos de `std` usan una implementación de plataforma. No hay API de sincronización soportada todavía.

| Opción | Selección |
| --- | --- |
| Sin `--platform`, destino hosted | Linux x86_64 |
| Sin `--platform`, destino kernel | `none`: solo módulos que no requieren un OS |
| `--platform linux-x86_64` | Backend Linux; incompatible con el perfil kernel |
| `--platform none` | Rechaza imports que necesitan una plataforma |
| `--platform custom --platform-root DIR` | Backend externo; sin fallback a Linux |

`--platform-root` solo se acepta con `custom`. La raíz puede estar fuera del repositorio y contener espacios. `report.json` registra la plataforma y los hashes de las fuentes, incluidos los módulos externos. El ejecutable GSL también acepta estas opciones y `--list-sources`, que lista el conjunto de imports sin emitir IR.

### Archivos y E/S

- `std/fs`: `fs_open_read`, `fs_create_new`, `fs_close`, `fs_read_into` y las operaciones de `std/io`. `std/file` contiene solo apertura y cierre, para consumidores que no necesitan el helper de lectura completa.
- `std/io`: `io_read`, `io_write`, `io_write_all`, `io_print`, `io_println`, `io_eprintln`.
- `std/process`: `process_exit`.

`OpenResult` contiene `file` y `error`. `File` no es copiable; su handle y estado son internos por contrato (GSL aún no tiene campos privados). Apertura fallida entrega un archivo cerrado. Las operaciones toman `*File`; `fs_close` invalida el archivo incluso si falla y un segundo cierre devuelve `IO_CLOSED`. No hay cierre automático.

`IoResult` contiene `count: usize`, `error: u32` y `eof: bool`. Los errores propios se definen en [std/types.gsl](lib/std/types.gsl): `IO_OK`, `IO_INVALID`, `IO_CLOSED`, `IO_NOT_FOUND`, `IO_PERMISSION`, `IO_EXISTS`, `IO_DIRECTORY`, `IO_NO_SPACE`, `IO_INTERRUPTED`, `IO_WOULD_BLOCK`, `IO_UNSUPPORTED`, `IO_BUFFER_SMALL` e `IO_ERROR`. No son valores de errno ni usan estado global.

```gsl
import "std/fs"

func main() u32 {
    var opened = fs_open_read("input.bin")
    if opened.error != IO_OK { return opened.error }
    var buffer: [256]u8
    let part = io_read(&opened.file, &buffer[0], 256 as usize)
    let closed = fs_close(&opened.file)
    if part.error != IO_OK { return part.error }
    return closed
}
```

Los buffers son del llamador y deben ser válidos para la capacidad indicada; la biblioteca no retiene sus punteros ni asigna memoria. Las rutas y mensajes son strings terminados en NUL. `io_read`/`io_write` permiten transferencias parciales; cero bytes pedidos no toca el buffer ni indica EOF, después de validar el archivo. Una interrupción se devuelve al llamador en estas operaciones simples.

`io_write_all` reintenta interrupciones y avances parciales; ante error devuelve los bytes ya escritos. Una escritura sin progreso se informa como `IO_ERROR`; `IO_WOULD_BLOCK` no provoca un bucle de espera. `fs_read_into` cierra el archivo en todas sus rutas, conserva el prefijo leído y comprueba un byte adicional al llenar la capacidad: distingue tamaño exacto de `IO_BUFFER_SMALL`. Los helpers conservan el primer error, incluido el de cierre cuando no hubo otro. `fs_create_new` no reemplaza archivos existentes; Linux aplica umask a permisos iniciales 0666. No se promete atomicidad de las escrituras, ni persistencia en disco al cerrar.

Las funciones de impresión devuelven el resultado y no cierran stdout/stderr. La biblioteca no cambia handlers de señales; escribir a un pipe cerrado en Linux puede terminar el proceso por SIGPIPE si el programa no lo ignora o maneja.

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

### Migración desde 0.1.x

Sustituye `core/io` por `core/string` cuando solo necesites `strlen`, o por `std/io` para imprimir. `print`/`println`/`eprintln` pasan a `io_print`/`io_println`/`io_eprintln` y devuelven `IoResult`; `exit` pasa a `process_exit`. `print_raw` se reemplaza por escritura explícita de buffer. `open/read/write/close/write_string` y `O_*` dejan de ser API pública: usa los archivos y resultados anteriores. También cambian `core/arch` → `arch/x86_64/cpu`, `core/descriptors` → `arch/x86_64/descriptors`, `core/serial` → `devices/pc/serial` y `core/vga` → `devices/pc/vga`. No se mantienen aliases. Los módulos `platform/linux_x86_64/*` son implementación específica, no una API portable.

## Reconstruir y probar

El desarrollo del compilador conserva el controlador del repositorio. Las pruebas completas también requieren binutils y `qemu-system-x86_64`:

```bash
python3 scripts/gsl.py bootstrap --output /tmp/gsl-initial
python3 -m unittest discover -s test/bootstrap -p 'test_*.py' -v
python3 scripts/gsl.py test --compiler /tmp/gsl-initial/compiler \
  --output /tmp/gsl-acceptance
```

`test` reconstruye el compilador, compara el IR y los ejecutables de generaciones sucesivas, ejecuta fixtures y arranca las pruebas freestanding en QEMU. También altera entradas de forma reproducible: exige terminación en cinco segundos, diagnósticos válidos y conservación de la salida en los rechazos; LLVM comprueba el IR aceptado. Los casos generados se conservan en el directorio de resultados. Una referencia aritmética independiente en Python comprueba los resultados completos de programas enteros a `-O0` y `-O2`, incluidos casts y valores extremos; los traps y el orden de evaluación se verifican por separado. El compilador reconstruido queda en `/tmp/gsl-acceptance/candidate-b`.

La instalación se verifica desde un proyecto externo, después de mover el prefijo y eliminar el paquete descomprimido, con imports locales, `std/io` y un objeto freestanding.

La validación de publicación se ejecuta localmente. El workflow de GitHub Actions queda disponible solo mediante ejecución manual (`workflow_dispatch`); los pushes y pull requests no lo activan. Usa Ubuntu 24.04 y Clang/LLD 18.1.3 mediante [scripts/ci.sh](scripts/ci.sh). Para seleccionar esas versiones localmente, añade `--clang clang-18 --linker ld.lld-18 --llvm-version 18.1.3`. Las pruebas de errores de E/S requieren permitir `ptrace` sobre procesos hijos.

## Estado del lenguaje

El compilador GSL admite funciones, enteros de 8–64 bits, booleanos, strings terminados en NUL, variables, condicionales, bucles, structs normales y packed, punteros, arrays acotados, arenas, globals, secciones e intrínsecos de hardware. También admite comentarios de bloque anidados, retornos completos mediante `if/else` y asignaciones `/=`, `%=` y `>>=`. Los [casos de aceptación](test/bootstrap/acceptance.json) muestran el comportamiento verificado.

Arenas y move semantics forman parte del diseño de memoria; GSL comprueba movimientos de structs, copias explícitas y usos posteriores al movimiento. También rechaza movimientos de valores prestados y escapes conocidos de referencias locales o de arenas. El análisis es conservador; los escapes y las escrituras de referencias mediante punteros requieren `unsafe`.

Siguen pendientes `match`, `Option`/`Result`, `comptime`, concurrencia, floats y otras extensiones. Hay límites explícitos de almacenamiento y anidamiento; las capacidades están en [manifest.json](test/bootstrap/manifest.json). Los errores de compilación o escritura conservan la salida LLVM anterior. El destino debe ser un archivo regular o una ruta nueva.

Los errores semánticos muestran código, motivo y línea/columna cuando están disponibles. Con imports, `combined source` indica una posición en la fuente combinada, no en el archivo original. Las columnas cuentan bytes; el formato JSON del ejecutable del compilador conserva los spans en bytes.

La implementación activa está en [lib/compiler](lib/compiler/). La implementación Go fue retirada por completo; usa `scripts/gsl.py`. Los [contratos pospuestos](test/deferred/go_contracts.json) conservan ejemplos históricos con resultados esperados; son especificaciones inactivas, no capacidades ni pruebas aprobadas del compilador actual.
