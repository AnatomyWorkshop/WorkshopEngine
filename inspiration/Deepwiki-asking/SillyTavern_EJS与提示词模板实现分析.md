# SillyTavern EJS 与提示词模板实现分析

## 1. SillyTavern 中的 EJS 实现

### 1.1 EJS 的核心功能

**EJS**（Embedded JavaScript）是 SillyTavern 中用于动态生成内容的模板引擎，主要功能包括：

- **动态内容生成**：在模板中嵌入 JavaScript 代码
- **条件渲染**：使用 `<% if %>`、`<% else %>` 等条件语句
- **循环渲染**：使用 `<% for %>`、`<% while %>` 等循环语句
- **变量替换**：使用 `<%= variable %>` 输出变量值
- **宏指令支持**：在 EJS 中调用各种宏函数

### 1.2 EJS 的实现位置

根据代码分析，SillyTavern 中的 EJS 实现主要位于：

- **前端 EJS 渲染器**：在浏览器端实现，主要在 JS-Slash-Runner 中
- **世界书处理**：`public/scripts/world-info.js` 中处理世界书词条的 EJS 渲染
- **提示词模板**：`public/scripts/instruct-mode.js` 和 `public/scripts/PromptManager.js` 中处理提示词的 EJS 渲染

### 1.3 EJS 与宏系统的集成

SillyTavern 的宏系统与 EJS 紧密集成，主要通过以下方式：

1. **宏注册表**：`public/scripts/macros/macro-system.js` 中定义了完整的宏注册和管理系统
2. **宏引擎**：`MacroEngine` 负责解析和执行宏指令
3. **EJS 渲染**：EJS 模板中可以直接调用宏函数

### 1.4 核心实现代码

**宏系统核心**：
```javascript
// public/scripts/macros/macro-system.js
export const macros = {
    // engine singletons
    engine: MacroEngine,
    registry: MacroRegistry,
    envBuilder: MacroEnvBuilder,
    lexer: MacroLexer,
    parser: MacroParser,
    cstWalker: MacroCstWalker,

    // enums
    category: MacroCategory,

    // shorthand functions
    register: MacroRegistry.registerMacro.bind(MacroRegistry),
};
```

**EJS 渲染示例**：
```javascript
// 世界书词条中的 EJS 代码
<% if (_.includes(getvar('stat_data.世界状态.已经历剧情[0]') || [], "xxx")) { %>
    这是条件渲染的内容
<% } %>

// 提示词模板中的 EJS 代码
<%= allCharacterNames.join(', ') %>
```

## 2. 提示词模板插件

### 2.1 核心功能

SillyTavern 的提示词模板插件主要功能包括：

- **模板管理**：创建、编辑、删除提示词模板
- **模型适配**：根据不同模型自动选择合适的模板
- **变量替换**：在模板中使用变量和宏
- **条件逻辑**：使用 EJS 实现复杂的条件逻辑
- **模板导出/导入**：分享和使用社区模板

### 2.2 实现位置

- **模板管理**：`public/scripts/sysprompt.js`
- **模型绑定**：`public/scripts/chat-templates.js`
- **提示词构建**：`public/scripts/instruct-mode.js`
- **模板渲染**：`public/scripts/PromptManager.js`

### 2.3 核心实现代码

**模板推导**：
```javascript
// public/scripts/chat-templates.js
export async function deriveTemplatesFromChatTemplate(chat_template, hash) {
    if (chat_template.trim() === '') {
        console.log('Missing chat template.');
        return not_found;
    }

    if (hash in hash_derivations) {
        return parse_derivation(hash_derivations[hash]);
    }

    // heuristics
    for (const [derivation, substr] of substr_derivations) {
        if ([substr].flat().every(str => chat_template.includes(str))) {
            return parse_derivation(derivation);
        }
    }

    console.warn(`Unknown chat template hash: ${hash} for [${chat_template}]`);
    return not_found;
}
```

## 3. WE 中的宏注册表实现

### 3.1 WE 的宏注册表设计

WE 计划使用宏注册表来替代 EJS，主要特点：

- **中心化管理**：所有宏指令都注册到一个中央注册表中
- **类型安全**：为每个宏定义明确的参数类型和返回类型
- **模块化**：宏可以按功能分类，便于管理和扩展
- **性能优化**：预编译和缓存宏指令，提高执行效率

### 3.2 核心架构

WE 的宏注册表架构设计：

1. **宏定义**：每个宏都有唯一名称、参数列表、处理函数
2. **宏分类**：按功能分类（核心、环境、状态、聊天、时间、变量、指令）
3. **宏执行环境**：提供统一的执行上下文，包含必要的变量和函数
4. **宏解析器**：解析宏调用，提取参数，执行宏函数

### 3.3 实现示例

**宏注册示例**：
```javascript
// WE 宏注册表实现示例
class MacroRegistry {
    constructor() {
        this.macros = new Map();
    }

    registerMacro(name, options) {
        this.macros.set(name, {
            name,
            description: options.description,
            handler: options.handler,
            category: options.category,
            arguments: options.arguments || [],
            returnType: options.returnType || 'string'
        });
    }

    getMacro(name) {
        return this.macros.get(name);
    }

    executeMacro(name, args, context) {
        const macro = this.getMacro(name);
        if (!macro) {
            throw new Error(`Macro ${name} not found`);
        }
        return macro.handler(args, context);
    }
}
```

## 4. EJS 功能在 WE 中的实现

### 4.1 核心功能映射

| EJS 功能 | WE 宏注册表实现 |
|---------|---------------|
| 变量输出 `<%= var %>` | 变量宏 `{{var}}` |
| 条件渲染 `<% if %>` | 条件宏 `{{if}}` |
| 循环渲染 `<% for %>` | 循环宏 `{{for}}` |
| 函数调用 `<% func() %>` | 函数宏 `{{func}}` |
| 复杂逻辑 | 组合宏调用 |

### 4.2 实现策略

1. **基础变量替换**：
   - 实现 `{{var}}` 宏，直接输出变量值
   - 支持嵌套变量访问 `{{var.subvar}}`

2. **条件逻辑**：
   - 实现 `{{if condition}}content{{endif}}` 宏
   - 支持 `{{else}}` 和 `{{elseif}}` 分支

3. **循环逻辑**：
   - 实现 `{{for item in array}}content{{endfor}}` 宏
   - 支持循环索引 `{{index}}`

4. **函数调用**：
   - 实现通用函数调用宏 `{{func arg1 arg2}}`
   - 支持命名参数 `{{func name=value}}`

5. **模板包含**：
   - 实现 `{{include template}}` 宏，包含其他模板

### 4.3 实现代码示例

**条件宏实现**：
```javascript
// WE 条件宏实现
registry.registerMacro('if', {
    description: '条件渲染',
    handler: (args, context) => {
        const condition = args[0];
        const content = args[1];
        const elseContent = args[2] || '';
        
        return condition ? content : elseContent;
    },
    arguments: [
        { name: 'condition', type: 'boolean' },
        { name: 'content', type: 'string' },
        { name: 'elseContent', type: 'string', optional: true }
    ],
    returnType: 'string'
});
```

**循环宏实现**：
```javascript
// WE 循环宏实现
registry.registerMacro('for', {
    description: '循环渲染',
    handler: (args, context) => {
        const array = args[0];
        const template = args[1];
        let result = '';
        
        for (let i = 0; i < array.length; i++) {
            const itemContext = {
                ...context,
                item: array[i],
                index: i
            };
            result += template.replace(/\{\{item\}\}/g, array[i])
                             .replace(/\{\{index\}\}/g, i);
        }
        
        return result;
    },
    arguments: [
        { name: 'array', type: 'array' },
        { name: 'template', type: 'string' }
    ],
    returnType: 'string'
});
```

## 5. WE 宏注册表扩展实现 EJS 全部功能

### 5.1 技术可行性分析

WE 的宏注册表可以扩展实现 EJS 的全部功能，原因如下：

1. **表达能力**：宏注册表可以通过组合宏来实现复杂的逻辑，与 EJS 的 JavaScript 代码块功能等价
2. **扩展性**：宏注册表支持自定义宏，可以根据需要添加新的宏功能
3. **类型安全**：宏注册表可以为每个宏定义明确的类型，提高代码的可靠性
4. **性能优化**：宏注册表可以预编译和缓存宏指令，提高执行效率

### 5.2 实现挑战

1. **复杂逻辑处理**：EJS 中可以直接编写任意 JavaScript 代码，而宏注册表需要通过组合宏来实现类似功能
2. **变量作用域**：EJS 中的变量作用域规则与宏注册表的变量管理方式不同
3. **错误处理**：EJS 中的语法错误可以通过 JavaScript 引擎捕获，而宏注册表需要自己实现错误处理机制
4. **学习曲线**：开发者需要学习宏注册表的语法和使用方式，而不是直接使用熟悉的 JavaScript

### 5.3 解决方案

1. **增强宏功能**：
   - 实现更复杂的宏，支持更多的逻辑操作
   - 添加内置的数学运算、字符串处理等宏
   - 支持宏的嵌套调用

2. **变量管理**：
   - 实现变量作用域管理，支持局部变量和全局变量
   - 提供变量赋值宏 `{{set var value}}`
   - 支持变量类型转换

3. **错误处理**：
   - 实现宏执行错误的捕获和报告机制
   - 提供详细的错误信息，帮助开发者定位问题
   - 添加调试工具，支持宏执行过程的跟踪

4. **开发工具**：
   - 提供宏编辑器，支持语法高亮和自动补全
   - 实现宏预览功能，实时显示宏执行结果
   - 提供宏库，包含常用的宏模板

## 6. 性能比较

### 6.1 EJS 性能

- **优势**：
  - 直接使用 JavaScript 引擎执行，速度快
  - 成熟的模板引擎，优化完善
  - 语法简洁，易于理解

- **劣势**：
  - 需要解析和执行 JavaScript 代码，存在安全风险
  - 无法进行编译时优化
  - 错误处理依赖于 JavaScript 引擎

### 6.2 WE 宏注册表性能

- **优势**：
  - 可以进行编译时优化，提高执行效率
  - 类型安全，减少运行时错误
  - 模块化设计，易于维护和扩展
  - 安全可控，避免执行恶意代码

- **劣势**：
  - 宏解析和执行需要额外的处理步骤
  - 复杂逻辑的实现可能比 EJS 更繁琐
  - 学习成本较高

### 6.3 性能优化策略

1. **编译缓存**：
   - 预编译宏指令，避免重复解析
   - 缓存常用宏的执行结果
   - 使用记忆化技术优化频繁调用的宏

2. **执行优化**：
   - 批量处理宏指令，减少上下文切换
   - 使用高效的数据结构存储宏和变量
   - 实现惰性求值，只在需要时执行宏

3. **并行处理**：
   - 支持宏的并行执行，提高处理速度
   - 使用 Web Workers 处理复杂的宏计算

## 7. 结论

WE 的宏注册表设计可以扩展实现 EJS 的全部功能，并且在类型安全、性能优化和安全性方面具有优势。虽然在复杂逻辑处理和学习曲线方面存在挑战，但通过合理的设计和实现，可以克服这些挑战，构建一个功能强大、性能优良的模板系统。

### 7.1 推荐实现方案

1. **核心宏实现**：
   - 实现基础变量替换、条件逻辑、循环逻辑等核心宏
   - 提供常用的数学运算、字符串处理等辅助宏
   - 支持宏的组合和嵌套调用

2. **扩展机制**：
   - 设计开放的宏注册接口，允许第三方插件添加新宏
   - 实现宏的版本控制和兼容性管理
   - 提供宏的文档和示例

3. **工具支持**：
   - 开发宏编辑器和调试工具
   - 提供宏库和模板，方便开发者使用
   - 实现宏执行的性能分析工具

4. **迁移策略**：
   - 提供从 EJS 到宏注册表的迁移工具
   - 支持混合使用 EJS 和宏注册表
   - 逐步过渡到完全使用宏注册表

通过以上实现方案，WE 可以构建一个功能强大、性能优良的模板系统，满足复杂的游戏内容生成需求，同时保持代码的可维护性和安全性。