#!/usr/bin/env node

Error.stackTraceLimit = Infinity

const batchSubmitter = require('../dist/src/exec/run-batch-submitter')

batchSubmitter.run()
