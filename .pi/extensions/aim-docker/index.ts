/**
 * AIM Docker Data Stores Extension
 *
 * Project-local pi extension for inspecting and interacting with the Docker
 * Compose Kafka, Redis, and PostgreSQL services used by AIM.
 */

import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { StringEnum, Type } from "@earendil-works/pi-ai";
import {
	DEFAULT_MAX_BYTES,
	DEFAULT_MAX_LINES,
	defineTool,
	formatSize,
	truncateTail,
	type ExtensionAPI,
	type ExtensionContext,
	type ExecResult,
	type TruncationResult,
} from "@earendil-works/pi-coding-agent";
import { Text } from "@earendil-works/pi-tui";

let pi: ExtensionAPI;

const EXTENSION_STATUS_KEY = "aim-docker";
const DEFAULT_TIMEOUT_MS = 30_000;
const SHORT_TIMEOUT_MS = 8_000;

const SERVICES = {
	kafka: {
		service: "kafka",
		container: "aim-kafka",
	},
	redis: {
		service: "redis",
		container: "aim-redis",
	},
	postgres: {
		service: "postgres",
		container: "aim-postgres",
	},
} as const;

type StoreName = keyof typeof SERVICES;

interface CommandDetails {
	command: string;
	exitCode: number;
	stderr?: string;
	truncation?: TruncationResult;
	fullOutputPath?: string;
}

function dockerExecArgs(container: string, command: string[]): string[] {
	return ["exec", container, ...command];
}

function shellEscape(value: string): string {
	return `'${value.replace(/'/g, `'"'"'`)}'`;
}

function formatCommand(command: string, args: string[]): string {
	return [command, ...args.map((arg) => (/[\s'"$`\\]/.test(arg) ? shellEscape(arg) : arg))].join(" ");
}

function combineOutput(result: ExecResult): string {
	const parts: string[] = [];
	if (result.stdout.trim()) parts.push(result.stdout.trimEnd());
	if (result.stderr.trim()) parts.push(`[stderr]\n${result.stderr.trimEnd()}`);
	if (result.code !== 0) parts.push(`[exit code] ${result.code}`);
	return parts.join("\n\n") || "(no output)";
}

async function renderOutput(output: string, details: CommandDetails) {
	const truncation = truncateTail(output, {
		maxLines: DEFAULT_MAX_LINES,
		maxBytes: DEFAULT_MAX_BYTES,
	});

	let text = truncation.content;
	if (truncation.truncated) {
		const tempDir = await mkdtemp(join(tmpdir(), "pi-aim-docker-"));
		const tempFile = join(tempDir, "output.txt");
		await writeFile(tempFile, output, "utf8");

		details.truncation = truncation;
		details.fullOutputPath = tempFile;
		text += `\n\n[输出已截断：显示 ${truncation.outputLines}/${truncation.totalLines} 行`;
		text += `（${formatSize(truncation.outputBytes)}/${formatSize(truncation.totalBytes)}）。`;
		text += `完整输出已保存到：${tempFile}]`;
	}

	return {
		content: [{ type: "text" as const, text }],
		details,
	};
}

async function runDocker(
	pi: ExtensionAPI,
	ctx: ExtensionContext,
	args: string[],
	signal: AbortSignal | undefined,
	timeout = DEFAULT_TIMEOUT_MS,
) {
	return pi.exec("docker", args, {
		cwd: ctx.cwd,
		signal,
		timeout,
	});
}

async function runDockerForTool(
	pi: ExtensionAPI,
	ctx: ExtensionContext,
	args: string[],
	signal: AbortSignal | undefined,
	timeout = DEFAULT_TIMEOUT_MS,
) {
	const result = await runDocker(pi, ctx, args, signal, timeout);
	const output = combineOutput(result);
	const details: CommandDetails = {
		command: formatCommand("docker", args),
		exitCode: result.code,
		stderr: result.stderr.trim() || undefined,
	};

	if (result.code !== 0) {
		const rendered = await renderOutput(output, details);
		throw new Error(rendered.content[0].text);
	}

	return renderOutput(output, details);
}

async function serviceIsRunning(pi: ExtensionAPI, ctx: ExtensionContext, store: StoreName): Promise<boolean> {
	const result = await runDocker(
		pi,
		ctx,
		["inspect", "-f", "{{.State.Running}}", SERVICES[store].container],
		undefined,
		SHORT_TIMEOUT_MS,
	);
	return result.code === 0 && result.stdout.trim() === "true";
}

async function probeService(pi: ExtensionAPI, ctx: ExtensionContext, store: StoreName): Promise<string> {
	if (!(await serviceIsRunning(pi, ctx, store))) return "未运行";

	const result = await runDocker(
		pi,
		ctx,
		dockerExecArgs(SERVICES[store].container, healthCommand(store)),
		undefined,
		SHORT_TIMEOUT_MS,
	);

	return result.code === 0 ? "可连接" : "运行中/探测失败";
}

function healthCommand(store: StoreName): string[] {
	switch (store) {
		case "kafka":
			return ["/opt/kafka/bin/kafka-broker-api-versions.sh", "--bootstrap-server", "localhost:9092"];
		case "redis":
			return ["redis-cli", "ping"];
		case "postgres":
			return ["pg_isready", "-U", "user", "-d", "aim_auth"];
	}
}

const dockerStatusTool = defineTool({
	name: "aim_docker_status",
	label: "AIM Docker Status",
	description:
		"检查 AIM Docker Compose 中 Kafka、Redis、PostgreSQL 的运行与连接状态。输出会按 2000 行或 50KB 截断。",
	promptSnippet: "检查 AIM Docker Compose 数据存储（Kafka、Redis、PostgreSQL）的运行与连接状态",
	promptGuidelines: [
		"Use aim_docker_status before aim_kafka/aim_redis/aim_pg when the user asks to inspect Docker data stores and service availability is unclear.",
	],
	parameters: Type.Object({
		services: Type.Optional(
			Type.Array(StringEnum(["kafka", "redis", "postgres"] as const), {
				description: "要检查的服务；默认检查 kafka、redis、postgres。",
			}),
		),
	}),

	async execute(_toolCallId, params, signal, _onUpdate, ctx) {
		const services = (params.services?.length ? params.services : ["kafka", "redis", "postgres"]) as StoreName[];
		const compose = await runDocker(pi, ctx, ["compose", "ps", ...services.map((s) => SERVICES[s].service)], signal, SHORT_TIMEOUT_MS);

		const probes = await Promise.all(
			services.map(async (service) => `- ${service}: ${await probeService(pi, ctx, service)}`),
		);
		const output = [`[docker compose ps]`, combineOutput(compose), `[connection probes]`, ...probes].join("\n\n");

		return renderOutput(output, {
			command: formatCommand("docker", ["compose", "ps", ...services.map((s) => SERVICES[s].service)]),
			exitCode: compose.code,
			stderr: compose.stderr.trim() || undefined,
		});
	},

	renderCall(args, theme) {
		const services = args.services?.length ? args.services.join(", ") : "kafka, redis, postgres";
		return new Text(`${theme.fg("toolTitle", theme.bold("aim_docker_status "))}${theme.fg("accent", services)}`, 0, 0);
	},

	renderResult(result, { expanded, isPartial }, theme) {
		if (isPartial) return new Text(theme.fg("warning", "检查 Docker 数据存储状态中..."), 0, 0);
		const details = result.details as CommandDetails | undefined;
		let text = theme.fg(details?.exitCode === 0 ? "success" : "error", details?.exitCode === 0 ? "✓ Docker 数据存储状态已检查" : "✗ Docker 状态检查失败");
		if (details?.truncation?.truncated) text += theme.fg("warning", "（输出已截断）");
		if (expanded) {
			const content = result.content[0];
			if (content?.type === "text") text += `\n${theme.fg("dim", content.text)}`;
		}
		return new Text(text, 0, 0);
	},
});

const kafkaTool = defineTool({
	name: "aim_kafka",
	label: "AIM Kafka",
	description:
		"连接 Docker 中的 AIM Kafka（容器 aim-kafka），支持 list_topics、describe_topic、consume、list_groups、describe_group。输出会按 2000 行或 50KB 截断。",
	promptSnippet: "通过 Docker 连接 AIM Kafka，列出/描述 topic、消费消息、查看 consumer group",
	promptGuidelines: [
		"Use aim_kafka for Kafka inspection instead of raw docker exec when working with AIM's local Docker Compose Kafka.",
		"Use aim_kafka consume with a small maxMessages value unless the user explicitly asks for more messages.",
	],
	parameters: Type.Object({
		action: StringEnum(["list_topics", "describe_topic", "consume", "list_groups", "describe_group"] as const, {
			description: "Kafka 操作。",
		}),
		topic: Type.Optional(Type.String({ description: "topic 名称；describe_topic/consume 必填。" })),
		group: Type.Optional(Type.String({ description: "consumer group 名称；describe_group 必填。" })),
		fromBeginning: Type.Optional(Type.Boolean({ description: "consume 时是否从头读取；默认 false。" })),
		maxMessages: Type.Optional(Type.Number({ description: "consume 最多读取消息数；默认 10。" })),
		timeoutMs: Type.Optional(Type.Number({ description: "命令超时时间；默认 30000ms。" })),
	}),

	async execute(_toolCallId, params, signal, _onUpdate, ctx) {
		const args: string[] = [];
		const kafkaBin = "/opt/kafka/bin";
		const bootstrap = ["--bootstrap-server", "localhost:9092"];

		switch (params.action) {
			case "list_topics":
				args.push(`${kafkaBin}/kafka-topics.sh`, ...bootstrap, "--list");
				break;
			case "describe_topic":
				if (!params.topic) throw new Error("describe_topic 需要 topic 参数");
				args.push(`${kafkaBin}/kafka-topics.sh`, ...bootstrap, "--describe", "--topic", params.topic);
				break;
			case "consume": {
				if (!params.topic) throw new Error("consume 需要 topic 参数");
				const maxMessages = Math.max(1, Math.min(500, Math.trunc(params.maxMessages ?? 10)));
				args.push(
					`${kafkaBin}/kafka-console-consumer.sh`,
					...bootstrap,
					"--topic",
					params.topic,
					"--max-messages",
					String(maxMessages),
					"--timeout-ms",
					String(Math.max(1000, Math.trunc(params.timeoutMs ?? DEFAULT_TIMEOUT_MS))),
				);
				if (params.fromBeginning) args.push("--from-beginning");
				break;
			}
			case "list_groups":
				args.push(`${kafkaBin}/kafka-consumer-groups.sh`, ...bootstrap, "--list");
				break;
			case "describe_group":
				if (!params.group) throw new Error("describe_group 需要 group 参数");
				args.push(`${kafkaBin}/kafka-consumer-groups.sh`, ...bootstrap, "--describe", "--group", params.group);
				break;
		}

		return runDockerForTool(
			pi,
			ctx,
			dockerExecArgs(SERVICES.kafka.container, args),
			signal,
			Math.max(1000, Math.trunc(params.timeoutMs ?? DEFAULT_TIMEOUT_MS)) + 5_000,
		);
	},

	renderCall(args, theme) {
		let text = theme.fg("toolTitle", theme.bold("aim_kafka ")) + theme.fg("accent", args.action ?? "");
		if (args.topic) text += theme.fg("muted", ` topic=${args.topic}`);
		if (args.group) text += theme.fg("muted", ` group=${args.group}`);
		return new Text(text, 0, 0);
	},

	renderResult(result, { expanded, isPartial }, theme) {
		if (isPartial) return new Text(theme.fg("warning", "连接 Kafka 中..."), 0, 0);
		const details = result.details as CommandDetails | undefined;
		let text = theme.fg(details?.exitCode === 0 ? "success" : "error", details?.exitCode === 0 ? "✓ Kafka 命令完成" : "✗ Kafka 命令失败");
		if (details?.truncation?.truncated) text += theme.fg("warning", "（输出已截断）");
		if (expanded) {
			const content = result.content[0];
			if (content?.type === "text") text += `\n${theme.fg("dim", content.text)}`;
		}
		return new Text(text, 0, 0);
	},
});

const redisTool = defineTool({
	name: "aim_redis",
	label: "AIM Redis",
	description:
		"连接 Docker 中的 AIM Redis（容器 aim-redis），支持 ping、info、dbsize、scan、type、ttl、get、set、del。输出会按 2000 行或 50KB 截断。",
	promptSnippet: "通过 Docker 连接 AIM Redis，执行常用 redis-cli 检查/读写命令",
	promptGuidelines: [
		"Use aim_redis for Redis inspection instead of raw docker exec when working with AIM's local Docker Compose Redis.",
		"Use aim_redis scan rather than KEYS for key discovery to avoid blocking Redis.",
	],
	parameters: Type.Object({
		action: StringEnum(["ping", "info", "dbsize", "scan", "type", "ttl", "get", "set", "del"] as const, {
			description: "Redis 操作。",
		}),
		database: Type.Optional(Type.Number({ description: "Redis DB 编号；默认 0。" })),
		key: Type.Optional(Type.String({ description: "key；type/ttl/get/set/del 必填。" })),
		value: Type.Optional(Type.String({ description: "set 写入的字符串值。" })),
		pattern: Type.Optional(Type.String({ description: "scan 匹配模式；默认 *。" })),
	}),

	async execute(_toolCallId, params, signal, _onUpdate, ctx) {
		const db = String(Math.max(0, Math.trunc(params.database ?? 0)));
		const baseArgs = ["redis-cli", "-n", db];
		const args: string[] = [...baseArgs];

		switch (params.action) {
			case "ping":
				args.push("PING");
				break;
			case "info":
				args.push("INFO");
				break;
			case "dbsize":
				args.push("DBSIZE");
				break;
			case "scan":
				args.push("--scan", "--pattern", params.pattern ?? "*");
				break;
			case "type":
			case "ttl":
			case "get":
			case "del":
				if (!params.key) throw new Error(`${params.action} 需要 key 参数`);
				args.push(params.action.toUpperCase(), params.key);
				break;
			case "set":
				if (!params.key) throw new Error("set 需要 key 参数");
				if (params.value === undefined) throw new Error("set 需要 value 参数");
				args.push("SET", params.key, params.value);
				break;
		}

		return runDockerForTool(pi, ctx, dockerExecArgs(SERVICES.redis.container, args), signal);
	},

	renderCall(args, theme) {
		let text = theme.fg("toolTitle", theme.bold("aim_redis ")) + theme.fg("accent", args.action ?? "");
		if (args.key) text += theme.fg("muted", ` key=${args.key}`);
		return new Text(text, 0, 0);
	},

	renderResult(result, { expanded, isPartial }, theme) {
		if (isPartial) return new Text(theme.fg("warning", "连接 Redis 中..."), 0, 0);
		const details = result.details as CommandDetails | undefined;
		let text = theme.fg(details?.exitCode === 0 ? "success" : "error", details?.exitCode === 0 ? "✓ Redis 命令完成" : "✗ Redis 命令失败");
		if (details?.truncation?.truncated) text += theme.fg("warning", "（输出已截断）");
		if (expanded) {
			const content = result.content[0];
			if (content?.type === "text") text += `\n${theme.fg("dim", content.text)}`;
		}
		return new Text(text, 0, 0);
	},
});

function assertPgIdentifier(name: string, label: string): string {
	if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
		throw new Error(`${label} 只能包含 PostgreSQL 普通标识符字符（字母、数字、下划线，且不能以数字开头）。复杂名称请改用 query。`);
	}
	return name;
}

const pgTool = defineTool({
	name: "aim_pg",
	label: "AIM PostgreSQL",
	description:
		"连接 Docker 中的 AIM PostgreSQL（容器 aim-postgres），支持 list_databases、list_tables、describe_table、query。query 默认只允许只读 SQL；确需写入时显式 readonly=false。输出会按 2000 行或 50KB 截断。",
	promptSnippet: "通过 Docker 连接 AIM PostgreSQL，列库/列表/描述表/执行 SQL 查询",
	promptGuidelines: [
		"Use aim_pg for PostgreSQL inspection instead of raw docker exec when working with AIM's local Docker Compose PostgreSQL.",
		"Use aim_pg query with readonly=true by default; only set readonly=false when the user explicitly asks to mutate local development data.",
	],
	parameters: Type.Object({
		action: StringEnum(["list_databases", "list_tables", "describe_table", "query"] as const, {
			description: "PostgreSQL 操作。",
		}),
		database: Type.Optional(Type.String({ description: "数据库名；默认 aim_auth。常用：aim_auth、aim_logic。" })),
		schema: Type.Optional(Type.String({ description: "schema；list_tables/describe_table 默认 public。" })),
		table: Type.Optional(Type.String({ description: "表名；describe_table 必填。" })),
		query: Type.Optional(Type.String({ description: "SQL；query 必填。默认只允许只读 SQL。" })),
		readonly: Type.Optional(Type.Boolean({ description: "query 是否只读保护；默认 true。" })),
	}),

	async execute(_toolCallId, params, signal, _onUpdate, ctx) {
		const database = params.database ?? "aim_auth";
		const schema = assertPgIdentifier(params.schema ?? "public", "schema");
		const baseArgs = ["psql", "-v", "ON_ERROR_STOP=1", "-X", "-U", "user"];
		const args: string[] = [];

		switch (params.action) {
			case "list_databases":
				args.push(...baseArgs, "-d", "postgres", "-c", "\\l");
				break;
			case "list_tables":
				args.push(...baseArgs, "-d", database, "-c", "\\dt " + schema + ".*");
				break;
			case "describe_table": {
				if (!params.table) throw new Error("describe_table 需要 table 参数");
				const table = assertPgIdentifier(params.table, "table");
				args.push(...baseArgs, "-d", database, "-c", "\\d+ " + schema + "." + table);
				break;
			}
			case "query": {
				if (!params.query) throw new Error("query 操作需要 query 参数");
				const sql = params.readonly === false ? params.query : `BEGIN READ ONLY; ${params.query}; COMMIT;`;
				args.push(...baseArgs, "-d", database, "-c", sql);
				break;
			}
		}

		return runDockerForTool(pi, ctx, dockerExecArgs(SERVICES.postgres.container, args), signal);
	},

	renderCall(args, theme) {
		let text = theme.fg("toolTitle", theme.bold("aim_pg ")) + theme.fg("accent", args.action ?? "");
		if (args.database) text += theme.fg("muted", ` db=${args.database}`);
		if (args.table) text += theme.fg("muted", ` table=${args.table}`);
		return new Text(text, 0, 0);
	},

	renderResult(result, { expanded, isPartial }, theme) {
		if (isPartial) return new Text(theme.fg("warning", "连接 PostgreSQL 中..."), 0, 0);
		const details = result.details as CommandDetails | undefined;
		let text = theme.fg(details?.exitCode === 0 ? "success" : "error", details?.exitCode === 0 ? "✓ PostgreSQL 命令完成" : "✗ PostgreSQL 命令失败");
		if (details?.truncation?.truncated) text += theme.fg("warning", "（输出已截断）");
		if (expanded) {
			const content = result.content[0];
			if (content?.type === "text") text += `\n${theme.fg("dim", content.text)}`;
		}
		return new Text(text, 0, 0);
	},
});

export default function (api: ExtensionAPI) {
	pi = api;

	api.registerTool(dockerStatusTool);
	api.registerTool(kafkaTool);
	api.registerTool(redisTool);
	api.registerTool(pgTool);

	api.on("session_start", async (_event, ctx) => {
		if (!ctx.hasUI) return;

		try {
			const statuses = await Promise.all(
				(["kafka", "redis", "postgres"] as StoreName[]).map(async (store) => {
					const running = await serviceIsRunning(pi, ctx, store);
					return `${store}:${running ? "up" : "down"}`;
				}),
			);
			ctx.ui.setStatus(EXTENSION_STATUS_KEY, ctx.ui.theme.fg("dim", `AIM Docker ${statuses.join(" ")}`));
		} catch {
			ctx.ui.setStatus(EXTENSION_STATUS_KEY, ctx.ui.theme.fg("warning", "AIM Docker unavailable"));
		}
	});

	api.registerCommand("aim-docker-status", {
		description: "检查 AIM Docker Compose 中 Kafka、Redis、PostgreSQL 的运行与连接状态",
		handler: async (_args, ctx) => {
			const lines = await Promise.all(
				(["kafka", "redis", "postgres"] as StoreName[]).map(async (store) => {
					return `${store}: ${await probeService(pi, ctx, store)}`;
				}),
			);

			ctx.ui.setWidget("aim-docker-status", ["AIM Docker 数据存储：", ...lines], { placement: "belowEditor" });
			ctx.ui.notify("AIM Docker 状态已更新到底部 widget", "info");
		},
	});
}
