import { describe, expect, it } from "vitest";
import { completionText, consumeSSEBuffer } from "./sse";

describe("consumeSSEBuffer", () => {
  it("retains partial events and parses complete events", () => {
    const result = consumeSSEBuffer('event: log\ndata: {"line":"one"}\n\ndata: partial');
    expect(result.messages).toEqual([{ event: "log", data: '{"line":"one"}' }]);
    expect(result.rest).toBe("data: partial");
  });

  it("extracts OpenAI stream content", () => {
    expect(completionText('{"choices":[{"delta":{"content":"hello"}}]}')).toBe("hello");
    expect(completionText("[DONE]")).toBe("");
  });
});
