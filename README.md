# GSL — Gio Systems Language

Compilador de un lenguaje de sistemas para Linux x86_64 y entornos freestanding x86_64. Está escrito en GSL y puede compilarse a sí mismo. Python organiza la compilación y LLVM genera el código nativo.

## Instalar y empezar

La distribución inicial requiere Linux x86_64 con glibc 2.34 o posterior, Python 3, Clang y LLD. Los objetos freestanding también requieren binutils. El entorno verificado es Ubuntu 24.04 con Clang/LLD 18.1.3; otras distros y musl todavía no están validadas. No requiere Go ni QEMU para compilar programas.

Descarga el paquete y `SHA256SUMS` desde [v0.1.0 experimental](https://github.com/GioTld/gslc/releases/tag/v0.1.0). Antes de extraerlo, verifica ambos archivos con `sha256sum -c SHA256SUMS`.

También puedes preparar el archivo de distribución desde este repositorio:

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
    println("Hola mundo")
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

La implementación activa está en [lib/compiler](lib/compiler/). La implementación Go fue retirada por completo; usa `scripts/gsl.py`. Los [contratos pospuestos](test/deferred/go_contracts.json) conservan ejemplos históricos con resultados esperados; son especificaciones inactivas, no capacidades ni pruebas aprobadas del compilador actual.
