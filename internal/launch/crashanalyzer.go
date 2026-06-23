package launch

import "strings"

func AnalyzeCrash(exitCode int, logs []LogEntry) *CrashAdvice {
	if exitCode == 0 {
		return nil
	}

	var lastLines []string
	start := len(logs) - 100
	if start < 0 {
		start = 0
	}
	for _, e := range logs[start:] {
		lastLines = append(lastLines, e.Line)
	}
	combined := strings.Join(lastLines, "\n")

	if a := checkByExitCode(exitCode); a != nil {
		return a
	}

	return analyzeLogs(exitCode, combined)
}

func checkByExitCode(exitCode int) *CrashAdvice {
	switch exitCode {
	case 1073740791: // 0xC0000409 - Stack buffer overrun / stack overflow
		return &CrashAdvice{
			Title:   "Критическая ошибка Java",
			Advice:  "Это ошибка связана с переполнением стека или повреждением памяти. Попробуйте: 1) Обновите Java до последней версии 2) Уменьшите выделение RAM в настройках 3) Отключите часть модов 4) Установите 64-битную версию Java",
			Details: "STATUS_STACK_BUFFER_OVERRUN (0xC0000409)",
		}
	case 1073741515: // 0xC000013B - STATUS_DLL_NOT_FOUND
		return &CrashAdvice{
			Title:   "Отсутствует системная библиотека",
			Advice:  "Не найдена системная DLL. Попробуйте: 1) Переустановите Java 2) Установите Microsoft Visual C++ Redistributable 3) Запустите лаунчер от имени администратора",
			Details: "STATUS_DLL_NOT_FOUND (0xC000013B)",
		}
	case 1073741819: // 0xC0000005 - ACCESS_VIOLATION
		return &CrashAdvice{
			Title:   "Ошибка доступа к памяти",
			Advice:  "Minecraft попытался обратиться к недоступной памяти. Попробуйте: 1) Уменьшите выделение RAM 2) Отключите недавно добавленные моды 3) Обновите Java 4) Проверьте целостность файлов игры (удалите версию и установите заново)",
			Details: "ACCESS_VIOLATION (0xC0000005)",
		}
	case 1073740940: // 0xC0000374 - HEAP_CORRUPTION
		return &CrashAdvice{
			Title:   "Повреждение кучи Java",
			Advice:  "Повреждена область памяти кучи. Попробуйте: 1) Уменьшите выделение RAM 2) Обновите Java 3) Проверьте совместимость модов 4) Переустановите версию Minecraft",
			Details: "HEAP_CORRUPTION (0xC0000374)",
		}
	case 3221226505: // 0xC0000139 - Entry point not found
		return &CrashAdvice{
			Title:   "Ошибка точки входа",
			Advice:  "Не найдена точка входа в библиотеке. Попробуйте: 1) Обновите Java 2) Переустановите Java 3) Обновите Windows",
			Details: "ENTRY_POINT_NOT_FOUND (0xC0000139)",
		}
	case 3:
		return &CrashAdvice{
			Title:   "Система не может запустить процесс",
			Advice:  "Проверьте: 1) Путь к Java в настройках лаунчера 2) Не заблокирован ли запуск антивирусом 3) Достаточно ли свободной оперативной памяти",
			Details: "ERROR_PATH_NOT_FOUND",
		}
	}
	return nil
}

func analyzeLogs(exitCode int, logs string) *CrashAdvice {
	logsLower := strings.ToLower(logs)

	if strings.Contains(logsLower, "out of memory") ||
		strings.Contains(logsLower, "outofmemoryerror") ||
		strings.Contains(logsLower, "not enough memory") ||
		strings.Contains(logsLower, "java.lang.outofmemoryerror") {
		return &CrashAdvice{
			Title:   "Не хватает оперативной памяти",
			Advice:  "Minecraft не хватило выделенной памяти. Откройте настройки и увеличьте значение 'Максимум RAM' (рекомендуется 2-4 ГБ для Fabric, 4-6 ГБ для модов). Если проблема не исчезнет — уменьшите количество модов.",
			Details: "OutOfMemoryError",
		}
	}

	if strings.Contains(logsLower, "could not reserve enough space") ||
		strings.Contains(logsLower, "could not reserve enough space for object heap") {
		return &CrashAdvice{
			Title:   "Система не может выделить память",
			Advice:  "У вас установлено слишком много RAM в настройках, и система не может выделить столько. Уменьшите 'Максимум RAM' в настройках лаунчера. Если у вас 32-битная Java — установите 64-битную.",
			Details: "CouldNotReserveHeap",
		}
	}

	if strings.Contains(logsLower, "could not create the java virtual machine") {
		return &CrashAdvice{
			Title:   "Неверные аргументы JVM",
			Advice:  "Java не может запуститься с текущими JVM-аргументами. Проверьте настройки лаунчера — возможно, вы указали неверные аргументы JVM. Сбросьте их до стандартных.",
			Details: "InvalidJVMArgs",
		}
	}

	if strings.Contains(logsLower, "unsupportedclassversionerror") ||
		strings.Contains(logsLower, "major.minor version") ||
		strings.Contains(logsLower, "unable to load class") {
		return &CrashAdvice{
			Title:   "Несовместимая версия Java",
			Advice:  "Для этой версии Minecraft требуется другая версия Java. Minecraft 1.18+ требует Java 17, Minecraft 1.21+ требует Java 21. Установите правильную версию Java в настройках лаунчера.",
			Details: "UnsupportedClassVersionError",
		}
	}

	if strings.Contains(logsLower, "glfw error") ||
		strings.Contains(logsLower, "gl error") ||
		strings.Contains(logsLower, "opengl") ||
		strings.Contains(logsLower, "pixel format") ||
		strings.Contains(logsLower, "wgl") {
		return &CrashAdvice{
			Title:   "Ошибка графического драйвера",
			Advice:  "Проблема с графическим драйвером или OpenGL. Попробуйте: 1) Обновите драйверы видеокарты 2) Установите более старую версию драйверов 3) Убедитесь, что ваша видеокарта поддерживает OpenGL 3.2+ 4) Запустите лаунчер от имени администратора",
			Details: "GLError",
		}
	}

	if strings.Contains(logsLower, "unable to find any jdkes") ||
		strings.Contains(logsLower, "no java runtime") ||
		strings.Contains(logsLower, "java is not recognized") {
		return &CrashAdvice{
			Title:   "Java не найдена",
			Advice:  "Система не может найти Java. Установите Java с официального сайта (java.com) и укажите путь к ней в настройках лаунчера.",
			Details: "JavaNotFound",
		}
	}

	if strings.Contains(logsLower, "fabric-loader") &&
		(strings.Contains(logsLower, "classnotfound") ||
			strings.Contains(logsLower, "noclassdeffound") ||
			strings.Contains(logsLower, "mixin") ||
			strings.Contains(logsLower, "mod conflict") ||
			strings.Contains(logsLower, "incompatible")) {
		return &CrashAdvice{
			Title:   "Ошибка совместимости модов (Fabric)",
			Advice:  "Конфликт модов Fabric. Попробуйте: 1) Удалите недавно добавленные моды 2) Обновите Fabric API 3) Проверьте, что все моды совместимы с вашей версией Minecraft 4) Удалите папку '.fabric' в '.minecraft'",
			Details: "FabricModError",
		}
	}

	if strings.Contains(logsLower, "mod") &&
		(strings.Contains(logsLower, "error loading") ||
			strings.Contains(logsLower, "failed to load") ||
			strings.Contains(logsLower, "mixin error") ||
			strings.Contains(logsLower, "classloading")) {
		return &CrashAdvice{
			Title:   "Ошибка загрузки модов",
			Advice:  "Один или несколько модов не загрузились. Попробуйте: 1) Удалите недавно установленные моды 2) Проверьте совместимость модов с версией Minecraft 3) Обновите Fabric API / Forge 4) Удалите папку 'mods' и установите моды заново",
			Details: "ModLoadError",
		}
	}

	if strings.Contains(logsLower, "modresolution") ||
		strings.Contains(logsLower, "missing dependency") ||
		strings.Contains(logsLower, "dependency not") {
		return &CrashAdvice{
			Title:   "Отсутствует зависимость мода",
			Advice:  "Какому-то моду не хватает библиотеки-зависимости. Проверьте, что у вас установлены все требуемые моды (например, Fabric API, Malilib, другие библиотеки).",
			Details: "MissingDependency",
		}
	}

	if strings.Contains(logsLower, "java.lang.reflect.invocationtargetexception") {
		return &CrashAdvice{
			Title:   "Внутренняя ошибка Minecraft",
			Advice:  "Внутренняя ошибка при запуске. Проверьте: 1) Целостность файлов игры (удалите версию и установите заново) 2) Отключите все моды и включайте по одному 3) Проверьте логи на вкладке 'Логи'",
			Details: "InvocationTargetException",
		}
	}

	if strings.Contains(logsLower, "java.io.ioexception") &&
		strings.Contains(logsLower, "access is denied") {
		return &CrashAdvice{
			Title:   "Нет доступа к файлам",
			Advice:  "Лаунчеру не хватает прав для доступа к файлам. Запустите лаунчер от имени администратора или проверьте, не заблокирован ли доступ антивирусом.",
			Details: "AccessDenied",
		}
	}

	if strings.Contains(logsLower, "corrupted") ||
		(strings.Contains(logsLower, "invalid") && strings.Contains(logsLower, "jar")) {
		return &CrashAdvice{
			Title:   "Повреждённый файл",
			Advice:  "Один из файлов Minecraft повреждён. Удалите проблемную версию в лаунчере и установите её заново.",
			Details: "CorruptedFile",
		}
	}

	if exitCode != 0 {
		return &CrashAdvice{
			Title:   "Minecraft завершился с ошибкой",
			Advice:  "Проверьте логи игры на вкладке 'Логи'. Там будет подробная информация о причине ошибки. Если проблема появилась после установки модов — попробуйте их отключить.",
			Details: "",
		}
	}

	return nil
}
