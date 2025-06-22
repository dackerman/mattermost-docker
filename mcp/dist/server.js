import express from "express";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { z } from "zod";
import { randomUUID } from "crypto";
const app = express();
app.use(express.json());
// Create MCP server instance
const server = new McpServer({
    name: "hello-world-server",
    version: "1.0.0"
});
// Register a simple hello-world tool
server.registerTool("hello", {
    title: "Hello World Tool",
    description: "Returns a hello world greeting",
    inputSchema: {
        name: z.string().optional().describe("Name to greet")
    }
}, async ({ name = "World" }) => ({
    content: [{
            type: "text",
            text: `Hello, ${name}!`
        }]
}));
// Register a simple hello-world resource
server.registerResource("greeting", "greeting://hello", {
    title: "Hello World Greeting",
    description: "A simple hello world greeting resource",
    mimeType: "text/plain"
}, async (uri) => ({
    contents: [{
            uri: uri.href,
            text: "Hello, World! This is a simple MCP server."
        }]
}));
// Store transports by session ID for session management
const transports = {};
// Handle all MCP requests
app.all('/mcp', async (req, res) => {
    try {
        const sessionId = req.headers['mcp-session-id'];
        let transport;
        if (sessionId && transports[sessionId]) {
            // Reuse existing transport for this session
            transport = transports[sessionId];
        }
        else {
            // Create new transport
            transport = new StreamableHTTPServerTransport({
                sessionIdGenerator: () => randomUUID(),
                onsessioninitialized: (sessionId) => {
                    transports[sessionId] = transport;
                }
            });
            transport.onclose = () => {
                if (transport.sessionId) {
                    delete transports[transport.sessionId];
                }
            };
            await server.connect(transport);
        }
        await transport.handleRequest(req, res, req.body);
    }
    catch (error) {
        console.error('Error handling MCP request:', error);
        if (!res.headersSent) {
            res.status(500).json({
                jsonrpc: '2.0',
                error: {
                    code: -32603,
                    message: 'Internal server error',
                },
                id: null,
            });
        }
    }
});
const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
    console.log(`Hello World MCP Server listening on port ${PORT}`);
    console.log(`MCP endpoint: http://localhost:${PORT}/mcp`);
});
