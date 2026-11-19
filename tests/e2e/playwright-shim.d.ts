// Ambient TypeScript declarations for @playwright/test so E2E specs and playwright.config.ts
// type-check cleanly via frontend/node_modules/typescript even before @playwright/test is installed.
declare const process: { env: Record<string, string | undefined> };

declare module '@playwright/test' {
  export interface Route {
    request(): {
      method(): string;
      url(): string;
      headers(): Record<string, string>;
      postDataJSON(): unknown;
    };
    fulfill(response: {
      status?: number;
      contentType?: string;
      headers?: Record<string, string>;
      body?: string;
      json?: unknown;
    }): Promise<void>;
    continue(): Promise<void>;
    abort(): Promise<void>;
  }

  export interface Locator {
    first(): Locator;
    nth(index: number): Locator;
    locator(selector: string): Locator;
    click(): Promise<void>;
    fill(value: string): Promise<void>;
    selectOption(value: string): Promise<string[]>;
    getAttribute(name: string): Promise<string | null>;
    textContent(): Promise<string | null>;
    count(): Promise<number>;
  }

  export interface Page {
    goto(url: string): Promise<unknown>;
    route(
      url: string | RegExp,
      handler: (route: Route) => Promise<void> | void,
    ): Promise<void>;
    locator(selector: string): Locator;
    getByTestId(testId: string): Locator;
    getByRole(
      role: string,
      options?: { name?: string | RegExp },
    ): Locator;
    getByText(text: string | RegExp): Locator;
    url(): string;
    evaluate<R, Arg = void>(
      pageFunction: (arg: Arg) => R | Promise<R>,
      arg?: Arg,
    ): Promise<R>;
  }

  export interface LocatorAssertions {
    toBeVisible(): Promise<void>;
    toBeHidden(): Promise<void>;
    toBeDisabled(): Promise<void>;
    toBeEnabled(): Promise<void>;
    toContainText(expected: string | RegExp): Promise<void>;
    toHaveText(expected: string | RegExp): Promise<void>;
    toHaveAttribute(name: string, value: string | RegExp): Promise<void>;
    toHaveCount(count: number): Promise<void>;
    toHaveValue(value: string | RegExp): Promise<void>;
  }

  export interface PageAssertions {
    toHaveURL(url: string | RegExp): Promise<void>;
  }

  export interface GenericAssertions {
    toBe(expected: unknown): void;
    toEqual(expected: unknown): void;
    toContain(expected: unknown): void;
    toBeTruthy(): void;
  }

  export function expect(actual: Locator): LocatorAssertions;
  export function expect(actual: Page): PageAssertions;
  export function expect(actual: unknown): GenericAssertions;

  export interface TestFixtures {
    page: Page;
  }

  export interface TestType {
    (
      title: string,
      testFunction: (fixtures: TestFixtures) => Promise<void> | void,
    ): void;
    describe(title: string, callback: () => void): void;
    beforeEach(
      hookFunction: (fixtures: TestFixtures) => Promise<void> | void,
    ): void;
  }

  export const test: TestType;

  export interface PlaywrightTestConfig {
    testDir?: string;
    timeout?: number;
    fullyParallel?: boolean;
    retries?: number;
    reporter?: string;
    use?: {
      baseURL?: string;
      trace?: string;
      headless?: boolean;
    };
    webServer?: {
      command: string;
      url: string;
      reuseExistingServer?: boolean;
      timeout?: number;
    };
  }

  export function defineConfig(config: PlaywrightTestConfig): PlaywrightTestConfig;
}
