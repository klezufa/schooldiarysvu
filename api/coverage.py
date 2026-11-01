"""Какие операции API (метод + путь из openapi.yaml) вызывают Go-интеграционные тесты.

Запуск: python3 api/coverage.py [--strict]. С --strict код возврата 1, если есть непокрытые операции, — для CI.
Разбор эвристический: путь в вызове testClient.request(http.MethodX, …) может быть строкой, склейкой строк и
выражений, fmt.Sprintf или локальной переменной с такой строкой. GET также считается покрытым, если путь встречается
в тестах как строка (такие запросы часто идут через вспомогательные функции).
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent


def operations():
    result, current = [], None
    for line in (ROOT / 'api' / 'openapi.yaml').read_text(encoding='utf-8').splitlines():
        path = re.match(r'^  (/[^:]*):\s*$', line)
        if path:
            current = path.group(1)
            continue
        method = re.match(r'^    (get|post|put|patch|delete):', line)
        if method and current:
            result.append((method.group(1).upper(), current))
        if re.match(r'^\S', line):
            current = None
    return result


def path_argument(text, start):
    """Склеивает аргумент-путь до запятой верхнего уровня: строки как есть, выражения — как X."""
    out, depth, i = '', 0, start
    while i < len(text):
        ch = text[i]
        if ch in '"`':
            end = text.index(ch, i + 1)
            out += text[i + 1:end]
            i = end + 1
            continue
        if ch in '([{':
            depth += 1
        elif ch in ')]}':
            if depth == 0:
                break
            depth -= 1
        elif ch in (',', '\n') and depth == 0:
            break
        elif ch == '+' and depth == 0:
            out += 'X'
        i += 1
    if out.startswith('fmt.Sprintf('):
        out = out[len('fmt.Sprintf('):]
    return re.sub(r'%[sdv]', 'X', out)


def called_operations():
    calls, literals = set(), ''
    for file in ROOT.glob('internal/**/*_integration_test.go'):
        text = file.read_text(encoding='utf-8')
        literals += text
        variables = {m.group(1): path_argument(text, m.end()) for m in re.finditer(r'\b(\w+) :?= (?=fmt\.Sprintf\(|"/api/v1)', text)}
        for m in re.finditer(r'http\.Method(\w+),\s*', text):
            name = re.match(r'(\w+)\s*,', text[m.end():])
            argument = variables.get(name.group(1), '') if name else path_argument(text, m.end())
            if '/api/v1' in argument:
                calls.add((m.group(1).upper(), argument[argument.index('/api/v1'):].split('?')[0]))
    return calls, literals


def pattern(path):
    return '^' + re.sub(r'\\{[^}]+\\}', '[^/]+', re.escape('/api/v1' + path)) + '$'


def main():
    sys.stdout.reconfigure(encoding='utf-8')
    calls, literals = called_operations()
    ops = operations()
    missing = []
    for method, path in ops:
        regex = pattern(path)
        hit = any(m == method and re.match(regex, c) for m, c in calls)
        if not hit and method == 'GET' and '{' not in path:
            hit = f'"/api/v1{path}' in literals
        if not hit:
            missing.append((method, path))
    print(f'Операций API: {len(ops)}, вызываются Go-интеграционными тестами: {len(ops) - len(missing)}')
    for method, path in missing:
        print(f'  не покрыто: {method} {path}')
    return 1 if missing and '--strict' in sys.argv else 0


if __name__ == '__main__':
    sys.exit(main())
