![canine banner](assets/jackal_logo.png)
# Canine Chain
**Canine Chain** is a core component to the Jackal Protocol, a distributed cloud storage platform running on blockchain rails. This component is the replicated state machine that manages storage deals, payments and data-permissions. It is built using the Cosmos-SDK and CometBFT (formerly Tendermint).

> **About this repository:** this is the Orbital Alchemists validator port of
> [JackalLabs/canine-chain](https://github.com/JackalLabs/canine-chain), based on
> upstream tag `v5.1.2`, carrying our Cosmos SDK v0.47 bridge-upgrade work on the
> `oa-stageB-part2` branch. Upstream history is preserved unchanged; the Jackal
> Protocol itself remains Jackal Labs' project. Building from this repository
> resolves our published IAVL cure fork (`orbitalalchemists/iavl-cure`), already
> pinned in `go.mod`.

## Wiki Pages

1. [Modules](x/README.md)
2. [Tokens](TOKENS.md)
3. [Storage Providers](cmd/canined/README.md)


## Installing the Canine CLI
### Prerequisites
* Install Go 1.23 [here](https://go.dev/dl/)
* Install build essentials (GNU Make)

### Installing
> if you want to use pebble follow this: https://github.com/JackalLabs/canine-chain/pull/511

To install `canined` on your Linux machine:

```shell
git clone https://github.com/orbitalalchemists/canine-chain.git
cd canine-chain
make install
```

## Testing this chain

```shell
make test
```

## License

Canine by Jackal uses the [MIT License](/LICENSE.md).

## Bug Bounty

Refer to the bug bounty program proposed by Jackal Labs [Here](https://jackaldao.medium.com/announcement-jackal-bug-bounty-program-31d4e03ab7e2)

### [Developer Contact](/ABOUT.md)
