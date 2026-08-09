import fs from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";

import esbuildPackage from "../web/classic/node_modules/esbuild/lib/main.js";
import postcss from "../web/default/node_modules/postcss/lib/postcss.mjs";
import tailwindPostcss from "../web/default/node_modules/@tailwindcss/postcss/dist/index.mjs";
import tailwindClassicPackage from "../web/classic/node_modules/tailwindcss/lib/index.js";

const repoRoot = path.resolve(import.meta.dirname, "..");
const moduleRoot = path.join(
  repoRoot,
  "examples",
  "extensions",
  "channel-quality",
);
const sourceRoot = path.join(moduleRoot, "native-src");
const outputRoot = path.join(moduleRoot, "public", "native");

const esbuild = esbuildPackage.default ?? esbuildPackage;
const tailwindClassic =
  tailwindClassicPackage.default ?? tailwindClassicPackage;

const sdkModule = (named, defaultExport = false) => ({
  defaultExport,
  named: new Set(named),
});

const targetConfig = {
  default: {
    entry: path.join(sourceRoot, "default", "entry.ts"),
    output: path.join(outputRoot, "default.mjs"),
    sdkModules: new Map([
      [
        "react",
        sdkModule(["Fragment", "useEffect", "useMemo", "useRef", "useState"]),
      ],
      ["react/jsx-runtime", sdkModule(["Fragment", "jsx", "jsxs"])],
      ["@tanstack/react-query", sdkModule(["useQuery"])],
      [
        "@hugeicons/core-free-icons",
        sdkModule([
          "Alert02Icon",
          "Analytics01Icon",
          "ArrowDown01Icon",
          "ArrowRight01Icon",
          "ChartRelationshipIcon",
          "Database01Icon",
          "FilterHorizontalIcon",
          "FilterResetIcon",
          "InformationCircleIcon",
          "Loading03Icon",
          "RefreshIcon",
          "Router01Icon",
          "Search01Icon",
          "TestTube01Icon",
        ]),
      ],
      ["@hugeicons/react", sdkModule(["HugeiconsIcon"])],
      ["react-i18next", sdkModule(["useTranslation"])],
      [
        "recharts",
        sdkModule(["CartesianGrid", "Line", "LineChart", "XAxis", "YAxis"]),
      ],
      ["sonner", sdkModule(["toast"])],
      ["@/lib/api", sdkModule(["api"])],
      ["@/lib/utils", sdkModule(["cn"])],
      ["@/components/layout", sdkModule(["SectionPageLayout"])],
      [
        "@/components/ui/alert",
        sdkModule(["Alert", "AlertDescription", "AlertTitle"]),
      ],
      ["@/components/ui/badge", sdkModule(["Badge"])],
      ["@/components/ui/button", sdkModule(["Button"])],
      [
        "@/components/ui/card",
        sdkModule([
          "Card",
          "CardContent",
          "CardDescription",
          "CardHeader",
          "CardTitle",
        ]),
      ],
      [
        "@/components/ui/chart",
        sdkModule([
          "ChartContainer",
          "ChartLegend",
          "ChartLegendContent",
          "ChartTooltip",
          "ChartTooltipContent",
        ]),
      ],
      [
        "@/components/ui/collapsible",
        sdkModule(["Collapsible", "CollapsibleContent"]),
      ],
      [
        "@/components/ui/empty",
        sdkModule([
          "Empty",
          "EmptyContent",
          "EmptyDescription",
          "EmptyHeader",
          "EmptyMedia",
          "EmptyTitle",
        ]),
      ],
      [
        "@/components/ui/field",
        sdkModule(["Field", "FieldGroup", "FieldLabel"]),
      ],
      ["@/components/ui/input", sdkModule(["Input"])],
      [
        "@/components/ui/input-group",
        sdkModule(["InputGroup", "InputGroupAddon", "InputGroupInput"]),
      ],
      [
        "@/components/ui/progress",
        sdkModule(["Progress", "ProgressLabel", "ProgressValue"]),
      ],
      [
        "@/components/ui/select",
        sdkModule([
          "Select",
          "SelectContent",
          "SelectGroup",
          "SelectItem",
          "SelectTrigger",
          "SelectValue",
        ]),
      ],
      ["@/components/ui/skeleton", sdkModule(["Skeleton"])],
      [
        "@/components/ui/table",
        sdkModule([
          "Table",
          "TableBody",
          "TableCell",
          "TableHead",
          "TableHeader",
          "TableRow",
        ]),
      ],
      [
        "@/components/ui/tabs",
        sdkModule(["Tabs", "TabsContent", "TabsList", "TabsTrigger"]),
      ],
      [
        "@/components/ui/toggle-group",
        sdkModule(["ToggleGroup", "ToggleGroupItem"]),
      ],
    ]),
  },
  classic: {
    entry: path.join(sourceRoot, "classic", "entry.jsx"),
    output: path.join(outputRoot, "classic.mjs"),
    sdkModules: new Map([
      [
        "react",
        sdkModule(
          [
            "Fragment",
            "useCallback",
            "useEffect",
            "useMemo",
            "useRef",
            "useState",
          ],
          true,
        ),
      ],
      ["react/jsx-runtime", sdkModule(["Fragment", "jsx", "jsxs"])],
      [
        "@douyinfe/semi-ui",
        sdkModule([
          "Banner",
          "Button",
          "ButtonGroup",
          "Card",
          "Collapsible",
          "DatePicker",
          "Empty",
          "Input",
          "Pagination",
          "Progress",
          "Select",
          "Space",
          "Spin",
          "Table",
          "Tabs",
          "Tag",
          "Toast",
          "Typography",
        ]),
      ],
      [
        "lucide-react",
        sdkModule([
          "Activity",
          "BadgeDollarSign",
          "CheckCircle2",
          "ChevronDown",
          "ChevronRight",
          "Clock3",
          "Database",
          "ExternalLink",
          "Layers3",
          "Play",
          "RefreshCw",
          "RotateCcw",
          "Search",
          "Server",
          "Sigma",
          "SlidersHorizontal",
          "TriangleAlert",
        ]),
      ],
      ["react-i18next", sdkModule(["useTranslation"])],
      ["@visactor/react-vchart", sdkModule(["VChart"])],
      ["@visactor/vchart-semi-theme", sdkModule(["initVChartSemiTheme"])],
      ["../../helpers", sdkModule(["API"])],
      ["../../constants/dashboard.constants", sdkModule(["CHART_CONFIG"])],
    ]),
  },
};

async function sourceFiles(root) {
  const result = [];
  for (const entry of await fs.readdir(root, { withFileTypes: true })) {
    const fullPath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      result.push(...(await sourceFiles(fullPath)));
    } else if (/\.(?:js|jsx|ts|tsx)$/.test(entry.name)) {
      result.push(fullPath);
    }
  }
  return result;
}

function collectClause(usage, clause) {
  const normalized = clause.replace(/\s+/g, " ").trim();
  if (!normalized || normalized.startsWith("type ")) return;

  if (/(?:^|,)\s*\*\s+as\s+/.test(normalized)) {
    usage.namespace = true;
    if (normalized.startsWith("*")) return;
  }

  const braceStart = normalized.indexOf("{");
  if (braceStart > 0) usage.default = true;
  if (braceStart < 0) usage.default = true;

  const match = normalized.match(/\{([\s\S]*)\}/);
  if (!match) return;
  for (const token of match[1].split(",")) {
    const item = token.trim();
    if (!item || item.startsWith("type ")) continue;
    const imported = item.split(/\s+as\s+/)[0].trim();
    if (/^[A-Za-z_$][\w$]*$/.test(imported)) usage.named.add(imported);
  }
}

async function collectImports(target, config) {
  const usageByModule = new Map();
  const root = path.dirname(config.entry);
  for (const file of await sourceFiles(root)) {
    const source = await fs.readFile(file, "utf8");
    const pattern = /import\s+(type\s+)?([\s\S]*?)\s+from\s+['"]([^'"]+)['"]/g;
    for (const match of source.matchAll(pattern)) {
      if (match[1] || !config.sdkModules.has(match[3])) continue;
      const usage = usageByModule.get(match[3]) ?? {
        default: false,
        namespace: false,
        named: new Set(),
      };
      collectClause(usage, match[2]);
      usageByModule.set(match[3], usage);
    }
  }
  const jsxUsage = usageByModule.get("react/jsx-runtime") ?? {
    default: false,
    namespace: false,
    named: new Set(),
  };
  for (const name of ["Fragment", "jsx", "jsxs"]) jsxUsage.named.add(name);
  usageByModule.set("react/jsx-runtime", jsxUsage);

  for (const external of config.sdkModules.keys()) {
    if (!usageByModule.has(external)) {
      usageByModule.set(external, {
        default: false,
        namespace: false,
        named: new Set(),
      });
    }
  }
  validateSdkUsage(target, config, usageByModule);
  return usageByModule;
}

function validateSdkUsage(target, config, usageByModule) {
  for (const [request, usage] of usageByModule) {
    const contract = config.sdkModules.get(request);
    if (!contract) {
      throw new Error(
        `Native extension SDK v1 for ${target} does not expose ${request}.`,
      );
    }
    if (usage.namespace) {
      throw new Error(
        `Native extension SDK imports must name explicit exports: ${request}.`,
      );
    }
    if (usage.default && !contract.defaultExport) {
      throw new Error(
        `Native extension SDK module has no default export: ${request}.`,
      );
    }
    for (const name of usage.named) {
      if (!contract.named.has(name)) {
        throw new Error(
          `Native extension SDK module ${request} does not export ${name}.`,
        );
      }
    }
  }
}

function sdkShim(target, request, usage, contract) {
  const namedExports = [...usage.named].sort();
  for (const name of namedExports) {
    if (!contract.named.has(name)) {
      throw new Error(
        `Native extension SDK module ${request} does not export ${name}.`,
      );
    }
  }
  const requiredExports = usage.default
    ? ["default", ...namedExports]
    : namedExports;
  const lines = [
    `const sdk = globalThis.__NEW_API_EXTENSION_NATIVE_SDK__;`,
    `if (!sdk || sdk.sdk !== 'v1' || sdk.platform !== ${JSON.stringify(target)}) {`,
    `  throw new Error('Native extension SDK v1 for ${target} is unavailable.');`,
    "}",
    `const hostModule = sdk.modules[${JSON.stringify(request)}];`,
    `if (!hostModule) throw new Error(${JSON.stringify(`Host SDK module is unavailable: ${request}`)});`,
  ];
  if (requiredExports.length > 0) {
    lines.push(
      `const missingExports = ${JSON.stringify(requiredExports)}.filter((name) => !(name in hostModule));`,
      `if (missingExports.length) throw new Error(${JSON.stringify(`Host SDK module ${request} is missing exports: `)} + missingExports.join(', '));`,
    );
  }
  if (usage.default) {
    lines.push("export default hostModule.default ?? hostModule;");
  }
  for (const name of namedExports) {
    lines.push(`export const ${name} = hostModule[${JSON.stringify(name)}];`);
  }
  return lines.join("\n");
}

function isPathInside(root, target) {
  const relative = path.relative(root, target);
  return (
    relative === "" ||
    (relative !== ".." &&
      !relative.startsWith(`..${path.sep}`) &&
      !path.isAbsolute(relative))
  );
}

function isBareImport(request) {
  return (
    !request.startsWith(".") &&
    !request.startsWith("/") &&
    !path.isAbsolute(request)
  );
}

async function buildTarget(target) {
  const config = targetConfig[target];
  const usageByModule = await collectImports(target, config);
  await esbuild.build({
    entryPoints: [config.entry],
    outfile: config.output,
    bundle: true,
    format: "esm",
    platform: "browser",
    target: ["es2020"],
    jsx: "automatic",
    minify: true,
    legalComments: "none",
    plugins: [
      {
        name: `new-api-native-sdk-${target}`,
        setup(build) {
          build.onResolve({ filter: /.*/ }, (args) => {
            if (config.sdkModules.has(args.path)) {
              return { path: args.path, namespace: "new-api-native-sdk" };
            }
            if (args.kind === "entry-point") return null;
            if (isBareImport(args.path)) {
              return {
                errors: [
                  {
                    text: `Native extension dependency is not exposed by SDK v1 for ${target}: ${args.path}`,
                  },
                ],
              };
            }
            if (args.path.startsWith("/") || path.isAbsolute(args.path)) {
              return {
                errors: [
                  {
                    text: `Native extension absolute imports are not allowed for ${target}: ${args.path}`,
                  },
                ],
              };
            }
            if (args.path.startsWith(".")) {
              const sourceDir = path.dirname(config.entry);
              const resolved = path.resolve(args.resolveDir, args.path);
              if (!isPathInside(sourceDir, resolved)) {
                return {
                  errors: [
                    {
                      text: `Native extension relative import escapes the ${target} source directory: ${args.path}`,
                    },
                  ],
                };
              }
            }
            return null;
          });
          build.onLoad(
            { filter: /.*/, namespace: "new-api-native-sdk" },
            (args) => ({
              contents: sdkShim(
                target,
                args.path,
                usageByModule.get(args.path),
                config.sdkModules.get(args.path),
              ),
              loader: "js",
            }),
          );
        },
      },
    ],
  });
}

async function buildDefaultStyles() {
  const webRoot = path.join(repoRoot, "web", "default");
  const from = path.join(webRoot, "src", "styles", "native-extension.css");
  const themePath = path.join(webRoot, "src", "styles", "theme.css");
  const themeRoot = postcss.parse(await fs.readFile(themePath, "utf8"), {
    from: themePath,
  });
  const inlineTheme = themeRoot.nodes.find(
    (node) =>
      node.type === "atrule" &&
      node.name === "theme" &&
      node.params.trim() === "inline",
  );
  if (!inlineTheme) {
    throw new Error(`Default theme mapping is missing: ${themePath}`);
  }
  const source = path
    .relative(path.dirname(from), path.join(sourceRoot, "default"))
    .replaceAll("\\", "/");
  const input = [
    '@import "tailwindcss/theme" layer(theme);',
    '@import "tailwindcss/utilities" layer(utilities) source(none);',
    inlineTheme.toString(),
    `@source "${source}";`,
  ].join("\n");
  const result = await postcss([tailwindPostcss()]).process(input, {
    from,
    map: false,
  });
  const requiredSelectors = [
    ".bg-muted\\/20",
    ".hover\\:bg-muted\\/30",
    ".border-primary\\/30",
    ".text-muted-foreground",
    ".text-destructive",
  ];
  for (const selector of requiredSelectors) {
    if (!result.css.includes(selector)) {
      throw new Error(`Default native style is missing: ${selector}`);
    }
  }
  if (result.css.includes("box-sizing: border-box")) {
    throw new Error("Default native style must not include Tailwind preflight");
  }
  await fs.writeFile(path.join(outputRoot, "default.css"), result.css);
}

async function buildClassicStyles() {
  const configModule = await import(
    pathToFileURL(path.join(repoRoot, "web", "classic", "tailwind.config.js"))
  );
  const config = {
    ...configModule.default,
    content: [path.join(sourceRoot, "classic", "**/*.{js,jsx}")],
  };
  const result = await postcss([tailwindClassic(config)]).process(
    "@tailwind utilities;",
    {
      from: path.join(sourceRoot, "classic", "native-extension.css"),
      map: false,
    },
  );
  await fs.writeFile(path.join(outputRoot, "classic.css"), result.css);
}

await fs.mkdir(outputRoot, { recursive: true });
await Promise.all([buildTarget("default"), buildTarget("classic")]);
await Promise.all([buildDefaultStyles(), buildClassicStyles()]);

for (const file of [
  "default.mjs",
  "classic.mjs",
  "default.css",
  "classic.css",
]) {
  const stat = await fs.stat(path.join(outputRoot, file));
  process.stdout.write(`${file}: ${stat.size} bytes\n`);
}
