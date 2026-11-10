+++
title = "SPDL: Lenguaje de definición de políticas Speedle"
description = "SPDL es Speedle Policy Definition Language, el lenguaje para escribir políticas y políticas de roles de Speedle."
weight = 5
draft = false
toc = true
singlelinetitle = true
tocheading = "h2"
tocsidebar = false
tags = ["v0.1.0", "core", "policy", "spdl"]
categories = ["docs"]
bref = "Conceptos básicos de SPDL"
lastmod = 2026-11-08T20:01:50+08:00
+++

SPDL es el lenguaje para escribir políticas y políticas de roles de Speedle. Esta página cubre la sintaxis, las reglas de nombres y los ejemplos.
<!--more-->

<aside class="docs-callout" aria-labelledby="spdl-callout-title">
  <p class="docs-callout-eyebrow">Modelador de políticas de Speedle · Habilidad de código abierto</p>
  <p class="docs-callout-title" id="spdl-callout-title">Escriba SPDL con IA</p>
  <p>La habilidad de código abierto del modelador de políticas de Speedle convierte sus requisitos y documentos de políticas en un modelo SPDL con casos de prueba de concesión y denegación, validados con <code>spctl check</code>.</p>
  <p class="docs-callout-actions"><a class="button primary" href="https://ai.speedle.io/es/#policy-modeler">Probar la habilidad del modelador de políticas</a> <a class="button secondary" href="https://github.com/teramoby/ai.speedle.io/tree/main/skills/speedle-policy-modeler">Código fuente en GitHub</a></p>
</aside>

## Sintaxis

<picture><source srcset="/img/speedle/spdl-syntax.webp" type="image/webp"><img src="/img/speedle/spdl-syntax.png" alt="Sintaxis básica de definición de políticas" width="2222" height="971" fetchpriority="high"></picture>

### Palabras clave

`grant`, `deny`, `user`, `group`, `entity`, `role`, `from`, `on`, `if` e `in` tienen un significado especial en su posición sintáctica. Las palabras clave de las sentencias no distinguen mayúsculas. En las condiciones, use `in` o `IN`; los nombres de función distinguen mayúsculas. Los nombres de principal, acción y recurso también distinguen mayúsculas. El analizador actual acepta nombres parecidos a palabras clave en algunas posiciones de nombre, así que una prohibición general no es una regla de validación. Prefiera nombres inequívocos para la revisión y la interoperabilidad de los clientes.

### Convención de nombres

Prefiera identificadores simples y estables. Escriba entre comillas los nombres con espacios en blanco o separadores, usando la forma de comillas simples o dobles del analizador, y pruebe la entrada real. Las clases de caracteres históricas de las expresiones regulares de Java no son un contrato de validación del analizador de Go. El prefijo de recurso `expr:` usa expresiones regulares de Go, no globs del shell. Los nombres de atributos y de funciones en las condiciones se describen más abajo.

### Sintaxis

<pre>
POLICY = EFFECT SUBJECT ACTION RESOURCE [if CONDITION]
EFFECT = grant | deny
SUBJECT = AND_PRINCIPALS (, AND_PRINCIPALS)*
AND_PRINCIPALS = PRINCIPAL | \( PRINCIPAL_LIST \)
PRINCIPAL_LIST = PRINCIPAL (, PRINCIPAL)*
PRINCIPAL = PRINCIPAL_TYPE PRINCIPAL_NAME [PRINCIPAL_IDD]
PRINCIPAL_TYPE = user|group|entity|role
PRINCIPAL_IDD = from IDD_IDENTIFIER
IDD_IDENTIFIER = NAME
ACTION = (ACTION_IDENTIFIER)(, ACTION_IDENTIFIER)*
RESOURCE = RESOURCE_IDENTIFIER
PRINCIPAL_NAME = NAME
ACTION_IDENTIFIER = NAME
RESOURCE_IDENTIFIER = NAME
</pre>
<pre>
ROLE_POLICY = EFFECT SUBJECT ROLE (on RESOURCE)? [if CONDITION]
EFFECT = grant | deny
SUBJECT = PRINCIPAL (, PRINCIPAL)*
PRINCIPAL = PRINCIPAL_TYPE PRINCIPAL_NAME [PRINCIPAL_IDD]
PRINCIPAL_TYPE = user|group|entity|role
PRINCIPAL_IDD = from IDD_IDENTIFIER
IDD_IDENTIFIER = NAME
ROLE = (role)? SUBJECT_IDENTIFIER
RESOURCE = RESOURCE_IDENTIFIER
SUBJECT_IDENTIFIER = NAME
RESOURCE_IDENTIFIER = NAME
</pre>

## Ejemplos

Herencia de roles: el sujeto de una política de roles puede ser otro rol, así que `manager` también tiene cada rol asignado a `employee`.

```text
grant role manager employee
```

La denegación prevalece sobre la concesión: una denegación aplicable gana a una concesión. Aquí `carol` está en `staff` y aun así se le deniega `delete` sobre `journal`.

```text
grant group staff read,delete journal
deny user carol delete journal
```

Una condición: la política se aplica solo cuando la condición es verdadera.

```text
grant user bob read report if department == 'finance'
```

Son sentencias SPDL, no un servicio de ejemplo completo; consulte [Gestión de políticas](../pms/policy-mgmt/) para los servicios, los roles con ámbito de recurso y ejemplos completos de decisión.

<a id="condition"></a>
## Condición

### 1. Descripción general

La condición se admite en las definiciones de políticas y de políticas de roles. La política o la política de roles surte efecto _solo_ cuando se cumple la condición.

El punto de aplicación de políticas (PEP) debe aportar cada atributo personalizado que referencien las políticas; los atributos integrados como `request_user` y `request_time` los proporciona ADS. Los atributos que aporta el PEP proceden de la aplicación o el servicio de confianza del llamador, no de los usuarios finales, y el PEP responde de los valores que envía. Los atributos aportados por el PEP tienen prioridad sobre los atributos integrados del mismo nombre; en particular, no reenvíe nunca entrada no confiable del usuario final como `request_user`, `request_time` ni ningún otro nombre de atributo integrado.

Una condición que referencia un atributo ausente en la solicitud no coincide. Por tanto, un `deny` condicional no se aplica cuando falta su atributo. Asegúrese de que el PEP aporta siempre los atributos que usan las condiciones de denegación, y pruebe esas solicitudes.

Este documento se centra en qué es una condición y en cómo escribirla.

### 2. Condición

Una condición es una expresión booleana que se construye con atributos, funciones, constantes, operadores, comparadores o paréntesis y produce un valor booleano.

#### 2.1 Tipos de datos

El tipo de dato de un valor de atributo y de una constante puede ser string, numeric, bool, datetime o un array de string, numeric, bool o datetime.  
Los tipos de datos y los operadores y comparadores admitidos se listan en la tabla siguiente.

 <table class="bordered striped">
    <thead>
      <tr>
        <th>Tipo de dato</th>
        <th>Operadores</th>
        <th>Comparadores</th>
        <th>Comentario</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>+</td>
        <td>== <br> != <br> =~ <br> > <br> >= <br> < <br> <= <br>
        </td>
        <td>'+' concatena cadenas <br> '=~' compara con una expresión regular; vea la nota bajo la tabla.</td>
      </tr>
      <tr>
        <td>numeric</td>
        <td> + <br> - <br> * <br> / <br> %</td>
        <td>== <br> != <br>  > <br> >= <br> < <br> <= </td>
        <td></td>
      </tr>
      <tr>
        <td>bool</td>
        <td>&& <br> || <br> !</td>
        <td>== <br>!=</td>
        <td></td>
      </tr>
      <tr>
        <td>datetime</td>
        <td></td>
        <td>== <br> != <br> > <br> >= <br> < <br> <=</td>
        <td></td>
      </tr>
      <tr>
        <td>array</td>
        <td></td>
        <td>in</td>
        <td>operador de pertenencia 'in': el lado izquierdo debe ser un tipo simple (string, numeric, bool, datetime) y el lado derecho un array</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

La mayoría de los comparadores se explican solos; esta es la nota de `"=~"`:  
`"=~"` es el comparador de expresiones regulares. Usa el dialecto regexp estándar de Go. El lado izquierdo es la cadena candidata; el lado derecho es el patrón.
`"=~"` indica si la cadena candidata coincide con el patrón de la derecha.

- Lado izquierdo: string
- Lado derecho: string
- Devuelve: bool

#### 2.2 Atributos

Un atributo en una condición representa una variable. El valor se determina en tiempo de ejecución y se obtiene del entorno del Servicio de Decisiones de Autorización (ADS) o del PEP.
Los atributos integrados son los valores que rellena el entorno de ADS.
Los atributos del cliente son los valores que aporta el PEP —la aplicación o el servicio de confianza del llamador— al pedir una decisión de autorización a ADS, no los usuarios finales. El PEP responde de los valores que envía. Los atributos aportados por el PEP tienen prioridad sobre los atributos integrados del mismo nombre, así que un PEP no debe reenviar entrada no confiable del usuario final con el nombre de un atributo integrado.

<a id="2-2-1-built-in-attributes"></a>
##### 2.2.1 Atributos integrados

Los atributos integrados son los siguientes:

<table class="bordered striped">
    <thead>
      <tr>
        <th>Nombre del atributo integrado</th>
        <th>Tipo de dato</th>
        <th>Valor de ejemplo</th>
        <th>Definición</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>request_user</td>
        <td>string</td>
        <td>"Alice"</td>
        <td>La información de usuario del sujeto que solicita actuar sobre un recurso</td>
      </tr>
      <tr>
        <td>request_groups</td>
        <td>[]string</td>
        <td>[]string{"managers"}</td>
        <td>La información de grupos del sujeto que solicita actuar sobre un recurso</td>
      </tr>
      <tr>
        <td>request_entity</td>
        <td>string</td>
        <td>"/org1/service1"</td>
        <td>El nombre de servicio u otra información de entidad del sujeto con el que el servicio o el programa solicita actuar sobre un recurso</td>
      </tr>
      <tr>
        <td>request_resource</td>
        <td>string</td>
        <td>"commercialLoans"</td>
        <td>El recurso sobre el que va a actuar el sujeto</td>
      </tr>
      <tr>
        <td>request_action</td>
        <td>string</td>
        <td>"issue" </td>
        <td>La acción que el sujeto va a realizar sobre un recurso </td>
      </tr>
      <tr>
        <td>request_time</td>
        <td>datetime</td>
        <td>'2019-01-02T15:04:05-07:00'</td>
        <td>La fecha y la hora en que ocurre la solicitud</td>
      </tr>
      <tr>
        <td>request_year</td>
        <td>int</td>
        <td>2019</td>
        <td>El año en que ocurre la solicitud</td>
      </tr>
      <tr>
        <td>request_month</td>
        <td>int</td>
        <td>1, 2, ... 12</td>
        <td>El mes en que ocurre la solicitud</td>
      </tr>
      <tr>
        <td>request_day</td>
        <td>int</td>
        <td>1, 2, ... 31</td>
        <td>El día del mes en que ocurre la solicitud</td>
      </tr>
      <tr>
        <td>request_hour</td>
        <td>int</td>
        <td>0, 1, ... 23</td>
        <td>La hora del día en que ocurre la solicitud</td>
      </tr>
      <tr>
        <td>request_weekday</td>
        <td>string</td>
        <td>"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"</td>
        <td>El día de la semana en que ocurre la solicitud</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

##### 2.2.2 Atributos del cliente

###### 2.2.2.1 name

Para referencias de condición portables, prefiera `[A-Za-z][A-Za-z0-9_]*` y evite los nombres de atributos y funciones integrados. La capa REST acepta hasta 256 letras ASCII, dígitos, guiones bajos, guiones y puntos. Pruebe las referencias de expresión que contienen un guion o un punto en lugar de suponer que son identificadores ordinarios y no operadores.

###### 2.2.2.2 value

Cuando una condición usa atributos del cliente, hay que pasar sus valores al solicitar un resultado isAllowed. Si la solicitud no lleva un atributo que la condición referencia, la condición no coincide; un `deny` condicional no se aplica en ese caso. Asegúrese de que el PEP aporta cada atributo que usan las condiciones de denegación, y pruebe esas solicitudes.

- Pasar valores de atributos en la API de Go  
   Siga estas reglas al pasar valores de atributos del cliente a una API de Go:

  - Use el tipo bool de Go para un valor bool.
  - Use el tipo string de Go para un valor string.
  - Use el tipo float64 de Go para cualquier valor numeric.
  - Para un valor datetime, use un float64 de Go con el tiempo Unix de esa fecha (mediante `time.Time.Unix()`).
  - Use []interface{} de Go para un valor array.

- Pasar valores de atributos en la API REST  
   Siga estas reglas al pasar valores de atributos del cliente a una API REST:
  - El atributo es una estructura con name, type y value. Consulte la API REST para el detalle.
  - El tipo del atributo solo puede ser "string", "numeric", "bool" o "datetime".
  - El valor del atributo puede ser un valor único o un slice.

#### 2.3 Constantes

Tipos de datos admitidos:

- string: comillas simples, 'foobar'
- numeric: 10, 3.1415926
- bool: true o false
- datetime: comillas simples, conforme a RFC3339. El formato RFC3339 es YYYY-MM-DDTHH:mm:SS[.sssssssss]Z, donde Z es [+|-]HH:mm.
- array: array de tipo string, numeric, bool o datetime

La tabla siguiente muestra constantes de ejemplo.

<table class="bordered striped">
    <thead>
      <tr>
        <th>Tipo de dato</th>
        <th>Constantes de ejemplo</th>
        <th>Arrays de ejemplo</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>string</td>
        <td>'a string'</td>
        <td>('string1', 'string2')</td>
      </tr>
      <tr>
        <td>bool</td>
        <td>true <br> false</td>
        <td>(true, false, true)</td>
      </tr>
      <tr>
        <td>numeric / float</td>
        <td>2.1</td>
        <td>(1, 2, 3.1)</td>
      </tr>
      <tr>
        <td>datetime</td>
        <td>'2006-01-02T15:04:05-07:00'</td>
        <td>('2016-01-02T15:04:05-07:00', '2019-01-02T15:04:05-07:00')</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

#### 2.4 Funciones

Use funciones cuando los operadores o comparadores existentes no cubren el requisito.

##### 2.4.1 Funciones integradas

Speedle proporciona las siguientes funciones integradas.

<table class="bordered striped">
    <thead>
      <tr>
        <th>Nombre de la función integrada</th>
        <th>Función</th>
        <th>Entrada y tipo de dato</th>
        <th>Salida y tipo de dato</th>
        <th>Uso de ejemplo</th>
      </tr>
    </thead>
    <tbody>
       <tr>
        <td>Sqrt</td>
        <td>Raíz cuadrada de un numeric</td>
        <td>Un parámetro numeric</td>
        <td>numeric</td>
        <td>Sqrt(x)<br>Sqrt(64)</td>
      </tr>
       <tr>
        <td>Max</td>
        <td>Obtiene el numeric máximo de un conjunto</td>
        <td>Uno o más parámetros numeric</td>
        <td>numeric</td>
        <td>Max(1, 4, x)</td>
      </tr>
       <tr>
        <td>Min</td>
        <td>Obtiene el numeric mínimo de un conjunto</td>
        <td>Uno o más parámetros numeric</td>
        <td>numeric</td>
        <td>Min(x, 5, z)</td>
      </tr>
       <tr>
        <td>Sum</td>
        <td>Obtiene la suma de un conjunto de numeric</td>
        <td>Uno o más parámetros numeric</td>
        <td>numeric</td>
        <td>Sum(1, 3, 5, 7, x)</td>
      </tr>
      <tr>
        <td>Avg</td>
        <td>Obtiene el promedio de un conjunto de numeric</td>
        <td>Uno o más parámetros numeric</td>
        <td>numeric</td>
        <td>Avg(x, 8, 10)</td>
      </tr>
      <tr>
        <td>IsSubSet</td>
        <td>Comprueba si el primer conjunto o array es subconjunto del segundo</td>
        <td>2 conjuntos o arrays cuyos elementos tienen el mismo tipo de dato</td>
        <td>bool</td>
        <td>IsSubSet(s1, s2)</td>
      </tr>
    </tbody>
    <tfoot>
    </tfoot>
  </table>

Las funciones numéricas integradas rechazan entradas o resultados no finitos y raíces cuadradas negativas con `BuiltInFuncError`. Una función que falla hace que su condición no coincida, igual que otros errores de condición en tiempo de ejecución. `Sum()` y `Avg()` conservan el resultado histórico de cero cuando la entrada está vacía. `IsSubSet` ignora los elementos duplicados, rechaza tipos de elemento no admitidos o mezclados, y conserva el resultado histórico false para un conjunto vacío. Los nombres entre comillas de SPDL conservan las barras invertidas literales; use el otro estilo de comillas cuando un nombre contiene una comilla. Las sentencias deben consumirse por completo: se rechazan el texto sobrante, las condiciones incompletas y las comillas de nombre sin cerrar.

##### 2.4.2 Funciones personalizadas

También se pueden exponer funciones propias mediante una API REST y usarlas en una expresión de condición.  
Para el detalle, consulte [función personalizada](../custom-function/).

#### 2.5 Precedencia de operadores y comparadores

##### 2.5.1 Orden de precedencia

Cuando dos operadores comparten un operando, se aplica primero el de mayor precedencia. Por ejemplo, `1 + 2 * 3` se trata como `1 + (2 * 3)`, mientras que `1 * 2 + 3` se trata como `(1 * 2) + 3` porque la multiplicación tiene mayor precedencia que la suma.

##### 2.5.2 Asociatividad

Cuando una expresión tiene dos operadores de la misma precedencia, se evalúa según su asociatividad. `72 / 2 / 3` se trata como `(72 / 2) / 3` porque el operador `/` es asociativo de izquierda a derecha. Algunos operadores no son asociativos: por ejemplo, las expresiones `(x <= y <= z)` y `x++--` no son válidas.

##### 2.5.3 Precedencia y asociatividad de los operadores y comparadores admitidos

La tabla siguiente resume la sintaxis de expresión documentada, de mayor a menor precedencia. Las comparaciones se ligan antes que el AND/OR lógico; use paréntesis para aclarar las condiciones complejas.

<table class="bordered striped"><thead><tr><th>Precedencia (de mayor a menor)</th><th>Operador o comparador</th><th>Asociatividad</th></tr></thead><tbody><tr><td>1</td><td>( )</td><td>N/A</td></tr><tr><td>2</td><td>llamada a función</td><td>N/A</td></tr><tr><td>3</td><td>! y - unario</td><td>De derecha a izquierda</td></tr><tr><td>4</td><td>* / %</td><td>De izquierda a derecha</td></tr><tr><td>5</td><td>+ -</td><td>De izquierda a derecha</td></tr><tr><td>6</td><td>== != =~ &gt; &gt;= &lt; &lt;= in</td><td>N/A</td></tr><tr><td>7</td><td>&amp;&amp;</td><td>De izquierda a derecha</td></tr><tr><td>8</td><td>||</td><td>De izquierda a derecha</td></tr></tbody></table>

#### 2.6 Condiciones de ejemplo

<table class="bordered striped">
    <thead>
      <tr>
        <th>Condición</th>
        <th>Comentario</th>
      </tr>
    </thead>
    <tbody>
    <tr><td>a=='abc'</td><td> El valor del atributo 'a' es igual a 'abc'</td></tr>
    <tr><td>a!='abc'</td><td></td></tr>
    <tr><td>a>='abc'</td><td></td></tr>
    <tr><td>a+b=='ab'</td><td></td></tr>
    <tr><td>a=~'^get.*'</td><td>El valor del atributo 'a' coincide con la expresión regular '^get.*'</td></tr>
<tr><td>a==123</td><td></td></tr>
<tr><td>a-b>123</td><td></td></tr>
<tr><td>a in (1, 2, 3) </td><td>El valor del atributo 'a' es uno de 1, 2, 3 </td></tr>
<tr><td>'manager' in a </td><td>'a' es un atributo de array de string y 'manager' es uno de sus elementos </td></tr>
<tr><td>IsSubSet(e, ('s1', 's2', 's3'))</td><td>'e' es un atributo de array o conjunto de string, y e es subconjunto de ('s1', 's2', 's3')</td></tr>
<tr><td>a in (1, 2, 3) && (b==c &#124;&#124; d==3) && IsSubSet(e, ('s1', 's2', 's3'))</td><td></td></tr>
<tr><td>request_year==2019 && request_month==12 </td><td> request_year y request_month son atributos integrados. El año en que se accede a un recurso es 2019 y el mes es 12</td></tr>
 </tbody>
    <tfoot>
    </tfoot>
  </table>

### Definición de la condición

La condición debe ser una expresión booleana válida. Lo siguiente resume las expresiones habituales; valide las entradas exactas con el analizador actual:

```
BoolExpr: ('!')BoolExpr
          | BoolExpr ('&&'|'||') BoolExpr
		  | BoolConstant
		  | Attribute
		  | Function
		  | RelationalExpr
		  | '(' BoolExpr ')'


RelationalExpr: NumericExpr ('=='|'!='|'>'|'>='|'<'|'<=') NumericExpr
              | StringExpr ('=='|'!='|'=~'|'>'|'>='|'<'|'<=') StringExpr
			  | BoolExpr ('=='|'!=') BoolExpr
			  | DateTimeExpr ('=='|'!='|'>'|'>='|'<'|'<=') DateTimeExpr
			  | (NumericExpr|StringExpr|BoolExpr|DateTimeExpr) ('in') ArrayExpr


NumericExpr: NumericExpr('+'|'-'|'*'|'/'|'%')NumericExpr
           | NumericConstant
		   | Attribute
		   | Function
		   | '(' NumericExpr ')'

StringExpr: StringExpr('+') StringExpr
           | StringConstant
		   | Attribute
		   | Function
           | '(' StringExpr ')'

DateTimeExpr: DateTimeConstant
           | Attribute
           | Function

ArrayExpr: ArrayConstant
         | Attribute
         | Function

ArrayConstant: '('Constant [, Constant]* ')'

Constant: NumericConstant
         |StringConstant
         |BoolConstant
         |DateTimeConstant

NumericConstant: any numeric float64 data
StringConstant: single quoted string, for example, 'string1'
BoolConstant: true|false
DateTimeConstant: single quoted datetime, datetime should conform to the format defined by RFC3339: YYYY-MM-DDTHH:mm:SS[.sssssssss]Z, Z is [+|-]HH:mm. For example, '2016-01-02T15:04:05-07:00'

Attribute: attribute name should conform to  [a-zA-Z]+[a-zA-Z0-9_]*, prefer simple portable identifiers; REST attribute names are limited to 256 ASCII characters
Function: FunctionName '(' Argument [,Argument]* ')'

```

## Apéndice

### Sintaxis completa de SPDL

<picture><source srcset="/img/speedle/spdl-syntax-full.webp" type="image/webp"><img src="/img/speedle/spdl-syntax-full.png" alt="Sintaxis completa de definición de políticas" width="1771" height="2281" loading="lazy"></picture>

<br/>
<br/>
