import { NodeOptions } from '@sentry/node'
import pino, { LevelWithSilent, LoggerOptions as PinoLoggerOptions } from 'pino'
import pinoms, { Streams } from 'pino-multi-stream'
import { createWriteStream } from 'pino-sentry'

export type LogLevel = LevelWithSilent

export interface LoggerOptions {
  name: string
  level?: LogLevel
  sentryOptions?: NodeOptions
  streams?: Streams
}

/**
 * Temporary wrapper class to maintain earlier module interface.
 */
export class Logger {
  options: LoggerOptions
  inner: pino.Logger

  constructor(options: LoggerOptions) {
    this.options = options

    const loggerOptions: PinoLoggerOptions = {
      name: options.name,

      level: options.level,

      // Remove pid and hostname considering production runs inside docker
      base: null,
    }

    let loggerStreams: Streams = [
      { stream: process.stdout, level: loggerOptions.level as LogLevel },
    ]
    if (options.sentryOptions) {
      loggerStreams.push({
        level: 'error',
        stream: createWriteStream({
          ...options.sentryOptions,
          stackAttributeKey: 'err',
        }),
      })
    }
    if (options.streams) {
      loggerStreams = loggerStreams.concat(options.streams)
    }

    this.inner = pino(loggerOptions, pinoms.multistream(loggerStreams))
  }

  child(bindings: pino.Bindings): Logger {
    const inner = this.inner.child(bindings)
    const logger = new Logger(this.options)
    logger.inner = inner
    return logger
  }

  trace(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      this.inner.trace(o, msg, ...args)
    } else {
      this.inner.trace(msg, ...args)
    }
  }

  debug(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      this.inner.debug(o, msg, ...args)
    } else {
      this.inner.debug(msg, ...args)
    }
  }

  info(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      this.inner.info(o, msg, ...args)
    } else {
      this.inner.info(msg, ...args)
    }
  }

  warn(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      this.inner.warn(o, msg, ...args)
    } else {
      this.inner.warn(msg, ...args)
    }
  }

  warning(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      this.inner.warn(o, msg, ...args)
    } else {
      this.inner.warn(msg, ...args)
    }
  }

  error(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      // Formatting error log for Sentry
      const context = {
        extra: { ...o },
      }
      this.inner.error(context, msg, ...args)
    } else {
      this.inner.error(msg, ...args)
    }
  }

  fatal(msg: string, o?: object, ...args: any[]): void {
    if (o) {
      const context = {
        extra: { ...o },
      }
      this.inner.fatal(context, msg, ...args)
    } else {
      this.inner.fatal(msg, ...args)
    }
  }
}
