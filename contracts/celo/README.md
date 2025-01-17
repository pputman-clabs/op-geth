# Celo contracts bytecode and ABI

## Why contracts in this repo?

The contracts bytecode is used to generate the genesis block in `geth --dev`
mode while the ABI is used to generate the contract bindings in `abigen`. The
bindings are necessary to access the FeeCurrencyDirectory and fee currencies.

## How to update to newer contracts

To compile contracts in the optimism repo and extract their ABI and bin-runtime
for relevant contracts into this repo, run `compiled/update.sh`. If your
optimism repo is not at `~/optimism`, set the `CELO_OPTIMISM_REPO` env variable
accordingly. The same is true for `~/celo-monorepo` and the `CELO_MONOREPO` env var.

## How to rebuild ABI wrappers

Use `go generate`, e.g. as `go generate ./contracts/celo/celo.go`.
