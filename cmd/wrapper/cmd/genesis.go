package cmd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	budgettypes "github.com/tendermint/budget/x/budget/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	claimtypes "github.com/crescent-network/crescent/x/claim/types"
	farmingtypes "github.com/crescent-network/crescent/x/farming/types"
	liquiditytypes "github.com/crescent-network/crescent/x/liquidity/types"
	liquidstakingtypes "github.com/crescent-network/crescent/x/liquidstaking/types"
	minttypes "github.com/crescent-network/crescent/x/mint/types"
)

var (
	// Airdrop result file
	filePath = "./data/result.csv"
)

func PrepareGenesisCmd(defaultNodeHome string, mbm module.BasicManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "prepare-genesis [network-type] [chain-id]",
		Args:    cobra.ExactArgs(2),
		Aliases: []string{"pg"},
		Short:   "Prepare a genesis file with initial setup",
		Long: strings.TrimSpace(
			fmt.Sprintf(`Prepare a genesis file with initial setup.

The initial setup includes initial params for Crescent

Example:
$ %s prepare-genesis mainnet crescent-1
$ %s prepare-genesis m crescent-1
$ %s prepare-genesis testnet mooncat-1-1
$ %s prepare-genesis t mooncat-1-1

The genesis output file is at $HOME/.crescent/config/genesis.json
`,
				version.AppName,
				version.AppName,
				version.AppName,
				version.AppName,
			),
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			serverCtx := server.GetServerContextFromCmd(cmd)

			serverCfg := serverCtx.Config

			// Read genesis file
			genFile := serverCfg.GenesisFile()
			appState, genDoc, err := genutiltypes.GenesisStateFromGenFile(genFile)
			if err != nil {
				return fmt.Errorf("failed to unmarshal genesis state: %w", err)
			}

			// Parse genesis params depending on the network type
			networkType := args[0]
			genStates := parseNetworkType(networkType)
			if genStates == nil {
				return fmt.Errorf("you must choose between mainnet (m) or testnet (t): %s", args[0])
			}

			// Prepare genesis
			chainID := args[1]
			appState, genDoc, err = PrepareGenesis(clientCtx, appState, genDoc, genStates, chainID)
			if err != nil {
				return fmt.Errorf("failed to prepare genesis %w", err)
			}

			// Validate genesis
			if err := mbm.ValidateGenesis(clientCtx.Codec, clientCtx.TxConfig, appState); err != nil {
				return fmt.Errorf("failed to validate genesis file: %w", err)
			}

			// Marshal and save the app state
			appStateJSON, err := json.Marshal(appState)
			if err != nil {
				return fmt.Errorf("failed to marshal application genesis state: %w", err)
			}
			genDoc.AppState = appStateJSON

			// Export the genesis state to a file
			if err := genutil.ExportGenesisFile(genDoc, genFile); err != nil {
				return fmt.Errorf("failed to export genesis file %w", err)
			}

			return err
		},
	}

	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "The application home directory")
	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

func PrepareGenesis(
	clientCtx client.Context,
	appState map[string]json.RawMessage,
	genDoc *tmtypes.GenesisDoc,
	genParams *GenesisStates,
	chainID string,
) (map[string]json.RawMessage, *tmtypes.GenesisDoc, error) {
	cdc := clientCtx.Codec

	genDoc.ChainID = chainID
	genDoc.GenesisTime = genParams.GenesisTime
	genDoc.ConsensusParams = genParams.ConsensusParams

	// Bank module app state
	bankGenState := banktypes.DefaultGenesisState()
	bankGenState.Balances = genParams.BankGenesisStates.Balances
	bankGenState.Supply = genParams.BankGenesisStates.Supply
	bankGenStateBz := cdc.MustMarshalJSON(bankGenState)
	appState[banktypes.ModuleName] = bankGenStateBz

	// Crisis module app state
	crisisGenState := crisistypes.DefaultGenesisState()
	crisisGenState.ConstantFee = genParams.CrisisStates.ConstantFee
	crisisGenStateBz := cdc.MustMarshalJSON(crisisGenState)
	appState[crisistypes.ModuleName] = crisisGenStateBz

	// Distribution module app state
	distrGenState := distrtypes.DefaultGenesisState()
	distrGenState.Params = genParams.DistributionParams
	distrGenStateBz := cdc.MustMarshalJSON(distrGenState)
	appState[distrtypes.ModuleName] = distrGenStateBz

	// Staking module app state
	stakingGenState := stakingtypes.DefaultGenesisState()
	stakingGenState.Params = genParams.StakingParams
	stakingGenStateBz := cdc.MustMarshalJSON(stakingGenState)
	appState[stakingtypes.ModuleName] = stakingGenStateBz

	// Mint module app state
	mintGenState := minttypes.DefaultGenesisState()
	mintGenState.Params = genParams.MintParams
	mintGenStateBz := cdc.MustMarshalJSON(mintGenState)
	appState[minttypes.ModuleName] = mintGenStateBz

	// Slashing module app state
	slashingGenState := slashingtypes.DefaultGenesisState()
	slashingGenState.Params = genParams.SlashingParams
	slashingGenStateBz := cdc.MustMarshalJSON(slashingGenState)
	appState[slashingtypes.ModuleName] = slashingGenStateBz

	// Gov module app state
	govGenState := govtypes.DefaultGenesisState()
	govGenState.DepositParams = genParams.GovParams.DepositParams
	govGenState.VotingParams = genParams.GovParams.VotingParams
	govGenState.TallyParams = genParams.GovParams.TallyParams
	govGenStateBz := cdc.MustMarshalJSON(govGenState)
	appState[govtypes.ModuleName] = govGenStateBz

	// Farming module app state
	farmingGenState := farmingtypes.DefaultGenesisState()
	farmingGenState.Params = genParams.FarmingParams
	farmingGenStateBz := cdc.MustMarshalJSON(farmingGenState)
	appState[farmingtypes.ModuleName] = farmingGenStateBz

	// Budget module app state
	budgetGenState := budgettypes.DefaultGenesisState()
	budgetGenState.Params = genParams.BudgetParams
	budgetGenStatebz := cdc.MustMarshalJSON(budgetGenState)
	appState[budgettypes.ModuleName] = budgetGenStatebz

	// Liquidstaking module app state
	liquidstakingGenState := liquidstakingtypes.DefaultGenesisState()
	liquidstakingGenState.Params = genParams.LiquidStakingParams
	liquidstakingGenStateBz := cdc.MustMarshalJSON(liquidstakingGenState)
	appState[liquidstakingtypes.ModuleName] = liquidstakingGenStateBz

	// Liquidity module app state
	liquidityGenState := liquiditytypes.DefaultGenesis()
	liquidityGenState.Params = genParams.LiquidityParams
	liquidityGenStateBz := cdc.MustMarshalJSON(liquidityGenState)
	appState[liquiditytypes.ModuleName] = liquidityGenStateBz

	// Claim module app state
	claimGenState := claimtypes.DefaultGenesis()
	claimGenState.Airdrops = genParams.ClaimGenesisState.Airdrops
	claimGenState.ClaimRecords = genParams.ClaimGenesisState.ClaimRecords
	claimGenStateBz := cdc.MustMarshalJSON(claimGenState)
	appState[claimtypes.ModuleName] = claimGenStateBz

	return appState, genDoc, nil
}

type GenesisStates struct {
	DEXdropSupply   sdk.Coin
	BoostdropSupply sdk.Coin
	BondDenom       string

	GenesisTime         time.Time
	ChainId             string
	ConsensusParams     *tmproto.ConsensusParams
	BankParams          banktypes.Params
	DistributionParams  distrtypes.Params
	StakingParams       stakingtypes.Params
	GovParams           govtypes.Params
	SlashingParams      slashingtypes.Params
	MintParams          minttypes.Params
	LiquidityParams     liquiditytypes.Params
	LiquidStakingParams liquidstakingtypes.Params
	FarmingParams       farmingtypes.Params
	BudgetParams        budgettypes.Params
	BankGenesisStates   banktypes.GenesisState
	CrisisStates        crisistypes.GenesisState
	ClaimGenesisState   claimtypes.GenesisState
}

func TestnetGenesisStates() *GenesisStates {
	genParams := &GenesisStates{}
	genParams.BondDenom = "ucre"
	genParams.DEXdropSupply = sdk.NewInt64Coin(genParams.BondDenom, 50_000_000_000_000)   // 50mil
	genParams.BoostdropSupply = sdk.NewInt64Coin(genParams.BondDenom, 50_000_000_000_000) // 50mil

	// Set genesis time
	genParams.GenesisTime = parseTime("2022-03-15T14:00:00Z")

	// Set consensus params
	genParams.ConsensusParams = &tmproto.ConsensusParams{
		Block: tmproto.BlockParams{
			MaxBytes:   10000000,
			MaxGas:     100000000,
			TimeIotaMs: 1000,
		},
		Evidence: tmproto.EvidenceParams{
			MaxAgeNumBlocks: 403200,
			MaxAgeDuration:  1209600000000000,
			MaxBytes:        1000000,
		},
		Validator: tmproto.ValidatorParams{
			PubKeyTypes: []string{"ed25519"},
		},
		Version: tmproto.VersionParams{},
	}

	// Set crisis
	genParams.CrisisStates = crisistypes.GenesisState{
		ConstantFee: sdk.NewInt64Coin(genParams.BondDenom, 1000),
	}

	// Set distribution params
	genParams.DistributionParams = distrtypes.Params{
		CommunityTax:        sdk.MustNewDecFromStr("0.285714285700000000"),
		BaseProposerReward:  sdk.MustNewDecFromStr("0.007142857143000000"),
		BonusProposerReward: sdk.MustNewDecFromStr("0.028571428570000000"),
		WithdrawAddrEnabled: true,
	}

	// Set staking params
	genParams.StakingParams = stakingtypes.Params{
		UnbondingTime:     1209600 * time.Second, // 2 weeks
		MaxValidators:     20,
		MaxEntries:        28,
		HistoricalEntries: 10000, // TODO
		BondDenom:         genParams.BondDenom,
	}

	// Set mint params
	genParams.MintParams = minttypes.Params{
		MintDenom:          genParams.BondDenom,
		BlockTimeThreshold: 10 * time.Second,
		InflationSchedules: []minttypes.InflationSchedule{
			{
				StartTime: genParams.GenesisTime,
				EndTime:   genParams.GenesisTime.AddDate(1, 0, 0),
				Amount:    sdk.NewInt(149400000000000),
			},
		},
	}

	// Set slashing params
	genParams.SlashingParams = slashingtypes.Params{
		SignedBlocksWindow:      10000,
		MinSignedPerWindow:      sdk.MustNewDecFromStr("0.050000000000000000"),
		DowntimeJailDuration:    600 * time.Second,
		SlashFractionDoubleSign: sdk.MustNewDecFromStr("0.050000000000000000"),
		SlashFractionDowntime:   sdk.MustNewDecFromStr("0.000100000000000000"),
	}

	// Set farming params
	genParams.FarmingParams = farmingtypes.DefaultParams()
	genParams.FarmingParams.PrivatePlanCreationFee = sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 100000000))

	// Set liquidity params
	genParams.LiquidityParams = liquiditytypes.Params{
		BatchSize:                1,
		TickPrecision:            3,
		FeeCollectorAddress:      "cre1zdew6yxyw92z373yqp756e0x4rvd2het37j0a2wjp7fj48eevxvq303p8d",
		DustCollectorAddress:     "cre1suads2mkd027cmfphmk9fpuwcct4d8ys02frk8e64hluswfwfj0s4xymnj",
		MinInitialPoolCoinSupply: sdk.NewInt(1000000000000),
		PairCreationFee:          sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 100000000)),
		PoolCreationFee:          sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 100000000)),
		MinInitialDepositAmount:  sdk.NewInt(100000000),
		DepositExtraGas:          sdk.Gas(60000),
		WithdrawExtraGas:         sdk.Gas(64000),
		OrderExtraGas:            sdk.Gas(37000),
		MaxPriceLimitRatio:       sdk.MustNewDecFromStr("0.100000000000000000"),
		MaxOrderLifespan:         86400 * time.Second,
		SwapFeeRate:              sdk.MustNewDecFromStr("0"),
		WithdrawFeeRate:          sdk.MustNewDecFromStr("0"),
	}

	// Set liquidstaking params
	genParams.LiquidStakingParams = liquidstakingtypes.Params{
		LiquidBondDenom: "ubcre",
		WhitelistedValidators: []liquidstakingtypes.WhitelistedValidator{
			// TODO: TBD
			{
				ValidatorAddress: "crevaloper1zaavvzxez0elundtn32qnk9lkm8kmcszyvldht", // alice operator address
				TargetWeight:     sdk.NewInt(10),
			},
		},
		UnstakeFeeRate:         sdk.MustNewDecFromStr("0"),
		MinLiquidStakingAmount: sdk.NewInt(1000000),
	}

	// Set gov params
	genParams.GovParams = govtypes.Params{
		DepositParams: govtypes.DepositParams{
			MinDeposit: sdk.NewCoins(
				sdk.NewInt64Coin(genParams.BondDenom, 500000000),
			),
			MaxDepositPeriod: 300 * time.Second,
		},
		VotingParams: govtypes.VotingParams{
			VotingPeriod: 300 * time.Second,
		},
		TallyParams: govtypes.TallyParams{
			Quorum:        sdk.MustNewDecFromStr("0.400000000000000000"),
			Threshold:     sdk.MustNewDecFromStr("0.500000000000000000"),
			VetoThreshold: sdk.MustNewDecFromStr("0.334000000000000000"),
		},
	}

	// Set budget params
	genParams.BudgetParams = budgettypes.Params{
		EpochBlocks: 1,
		Budgets: []budgettypes.Budget{
			{
				Name:               "budget-ecosystem-incentive",
				Rate:               sdk.MustNewDecFromStr("0.662500000000000000"),
				SourceAddress:      "cre17xpfvakm2amg962yls6f84z3kell8c5l53s97s",
				DestinationAddress: "cre1kgshua58cjr2p7hnrvgun68yrqf7ktdzyz2yxv54fqj6uwl4gc4q95txqa",
				StartTime:          genParams.GenesisTime,
				EndTime:            genParams.GenesisTime.AddDate(1, 0, 0),
			},
			{
				Name:               "budget-dev-team",
				Rate:               sdk.MustNewDecFromStr("0.250000000000000000"),
				SourceAddress:      "cre17xpfvakm2amg962yls6f84z3kell8c5l53s97s",
				DestinationAddress: "cre1z6utpv37rts2lytmwlft983yv3c5a2yy3utp8q",
				StartTime:          genParams.GenesisTime,
				EndTime:            genParams.GenesisTime.AddDate(1, 0, 0),
			},
		},
	}

	// Set airdrop
	genParams.ClaimGenesisState.Airdrops = []claimtypes.Airdrop{
		{
			Id:            1,
			SourceAddress: "cre15rz2rwnlgr7nf6eauz52usezffwrxc0mxajpmw", // TODO
			Conditions: []claimtypes.ConditionType{
				claimtypes.ConditionTypeDeposit,
				claimtypes.ConditionTypeSwap,
				claimtypes.ConditionTypeLiquidStake,
				claimtypes.ConditionTypeVote,
			},
			StartTime: genParams.GenesisTime,
			EndTime:   genParams.GenesisTime.AddDate(0, 6, 0),
		},
	}

	records, balances, totalInitialGenesisCoin := parseClaimRecords(genParams)

	// Set source account balances
	balances = append(balances, banktypes.Balance{
		Address: "cre15rz2rwnlgr7nf6eauz52usezffwrxc0mxajpmw", // source address
		Coins: sdk.NewCoins(
			genParams.DEXdropSupply.Add(genParams.BoostdropSupply),
		),
	})

	// Add custom accounts
	newBalances, totalCoins := addAccounts(genParams)
	balances = append(balances, newBalances...)

	// Set claim records and balances
	genParams.ClaimGenesisState.ClaimRecords = records
	genParams.BankGenesisStates.Balances = balances

	// Set total supply
	genParams.BankGenesisStates.Supply = sdk.NewCoins(
		genParams.DEXdropSupply.Add(genParams.BoostdropSupply).Add(totalInitialGenesisCoin),
	).Add(totalCoins...)

	genParams.BankParams = banktypes.Params{
		SendEnabled: []*banktypes.SendEnabled{
			{
				Denom:   genParams.BondDenom,
				Enabled: true,
			},
		},
		DefaultSendEnabled: true,
	}

	return genParams
}

func MainnetGenesisStates() *GenesisStates {
	genParams := &GenesisStates{}
	genParams.GenesisTime = parseTime("2022-04-14T00:00:00Z")

	// TODO: TBD

	return genParams
}

func addAccounts(genParams *GenesisStates) ([]banktypes.Balance, sdk.Coins) {
	balances := []banktypes.Balance{
		{
			Address: "cre1y4a8y4005ch3cx23f8alxpykuvtwh5stfcgutt",                             // foundation
			Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 100_000_000_000_000)), // 100mil
		},
		// {
		// 	Address: "cre1y4a8y4005ch3cx23f8alxpykuvtwh5stfcgutt", // multisig-foundation
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1z6utpv37rts2lytmwlft983yv3c5a2yy3utp8q", // multisig-devteam
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1s5cj0r5yhg7vdxmt6hsrzu60d3rdk9k6whnkf4", // foundation1
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1s9car3sthmaj273m7pju4wcaghg0s3rv6kt0s9", // foundation2
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1s8lhryggj6yvxhfa3dq072tftxp07uwtzv0vqr", // foundation3
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1je3rplrmx9fnfqxyu7nleufwwdt3e3kedn7z6u", // devteam1
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1gkvyqpj5sd6nz3c4jp6dzp4jlpl2m7c0vkp4t3", // devteam2
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
		// {
		// 	Address: "cre1yz4fsahrkamckmzv03sasgj95cquxntzxnchjg", // devteam3
		// 	Coins:   sdk.NewCoins(sdk.NewInt64Coin(genParams.BondDenom, 1)),
		// },
	}

	totalCoins := sdk.Coins{}
	for _, balance := range balances {
		totalCoins = totalCoins.Add(balance.Coins...)
	}

	return balances, totalCoins
}

func parseClaimRecords(genParams *GenesisStates) ([]claimtypes.ClaimRecord, []banktypes.Balance, sdk.Coin) {
	results, err := readCSVFile(filePath)
	if err != nil {
		panic(fmt.Sprintf("failed to read csv file %s", filePath))
	}

	totalInitialGenesisAmt := sdk.ZeroInt()
	balances := []banktypes.Balance{}
	records := []claimtypes.ClaimRecord{}

	for i, r := range results {
		if i == 0 {
			continue
		}

		recipientAddr := r[0]
		dexClaimableAmt, _ := sdk.NewIntFromString(r[1])

		_, converted, err := bech32.DecodeAndConvert(recipientAddr)
		if err != nil {
			panic(err)
		}

		targetPrefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
		recipientAddr, err = bech32.ConvertAndEncode(targetPrefix, converted)
		if err != nil {
			panic(err)
		}

		// Skip the zero amount
		if dexClaimableAmt.IsZero() {
			continue
		}

		initialGenesisAmt := dexClaimableAmt.Quo(sdk.NewInt(5))
		initialClaimableAmt := dexClaimableAmt.Sub(initialGenesisAmt)

		// 20% is set in genesis
		balances = append(balances, banktypes.Balance{
			Address: recipientAddr,
			Coins:   sdk.NewCoins(sdk.NewCoin(genParams.BondDenom, initialGenesisAmt)),
		})

		totalInitialGenesisAmt = totalInitialGenesisAmt.Add(initialGenesisAmt)

		// 80% is set in claim record
		records = append(records, claimtypes.ClaimRecord{
			AirdropId:             1,
			Recipient:             recipientAddr,
			InitialClaimableCoins: sdk.NewCoins(sdk.NewCoin(genParams.BondDenom, initialClaimableAmt)),
			ClaimableCoins:        sdk.NewCoins(sdk.NewCoin(genParams.BondDenom, initialClaimableAmt)),
		})
	}

	// (Testing) Set custom claim records
	records = append(records, claimtypes.ClaimRecord{
		AirdropId:             1,
		Recipient:             "cre1zaavvzxez0elundtn32qnk9lkm8kmcszxclz6p",
		InitialClaimableCoins: sdk.NewCoins(sdk.NewCoin(genParams.DEXdropSupply.Denom, sdk.NewInt(500_000_000_000))),
		ClaimableCoins:        sdk.NewCoins(sdk.NewCoin(genParams.DEXdropSupply.Denom, sdk.NewInt(500_000_000_000))),
		ClaimedConditions:     []claimtypes.ConditionType{},
	})

	return records, balances, sdk.NewCoin(genParams.BondDenom, totalInitialGenesisAmt)
}

// parseNetworkType returns GenesisStates based on the network type.
func parseNetworkType(networkType string) *GenesisStates {
	switch strings.ToLower(networkType) {
	case "t", "testnet":
		return TestnetGenesisStates()
	case "m", "mainnet":
		return MainnetGenesisStates()
	default:
		return nil
	}
}

// readCSVFile reads csv file and returns all the records.
func readCSVFile(filePath string) ([][]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	return records, nil
}

// parseTime parses and returns time.Time in time.RFC3339 format.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
