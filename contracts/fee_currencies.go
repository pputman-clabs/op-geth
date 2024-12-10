package contracts

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/exchange"
	"github.com/ethereum/go-ethereum/contracts/addresses"
	"github.com/ethereum/go-ethereum/contracts/celo/abigen"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
)

var feeCurrencyABI *abi.ABI

var ErrFeeCurrencyEVMCall = errors.New("fee-currency contract error during internal EVM call")

func init() {
	var err error
	feeCurrencyABI, err = abigen.FeeCurrencyMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
}

// Returns nil if debit is possible, used in tx pool validation
func TryDebitFees(tx *types.Transaction, from common.Address, backend *CeloBackend, feeContext common.FeeCurrencyContext) error {
	amount := new(big.Int).SetUint64(tx.Gas())
	amount.Mul(amount, tx.GasFeeCap())

	snapshot := backend.State.Snapshot()
	evm := backend.NewEVM(&feeContext)
	_, err := DebitFees(evm, tx.FeeCurrency(), from, amount)
	backend.State.RevertToSnapshot(snapshot)
	return err
}

// Debits transaction fees from the transaction sender and stores them in the temporary address
func DebitFees(evm *vm.EVM, feeCurrency *common.Address, address common.Address, amount *big.Int) (uint64, error) {
	// Hide this function from traces
	if evm.Config.Tracer != nil && !evm.Config.Tracer.TraceDebitCredit {
		origTracer := evm.Config.Tracer
		defer func() {
			evm.Config.Tracer = origTracer
		}()
		evm.Config.Tracer = nil
	}

	if amount.Cmp(big.NewInt(0)) == 0 {
		return 0, nil
	}

	maxIntrinsicGasCost, ok := common.MaxAllowedIntrinsicGasCost(evm.Context.FeeCurrencyContext.IntrinsicGasCosts, feeCurrency)
	if !ok {
		return 0, fmt.Errorf("%w: %x", exchange.ErrUnregisteredFeeCurrency, feeCurrency)
	}

	leftoverGas, err := evm.CallWithABI(
		feeCurrencyABI, "debitGasFees", *feeCurrency, maxIntrinsicGasCost,
		// debitGasFees(address from, uint256 value) parameters
		address, amount,
	)
	if err != nil {
		if errors.Is(err, vm.ErrOutOfGas) {
			// This basically is a configuration / contract error, since
			// the contract itself used way more gas than was expected (including grace limit)
			return 0, fmt.Errorf(
				"%w: surpassed maximum allowed intrinsic gas for DebitFees() in fee-currency: %w",
				ErrFeeCurrencyEVMCall,
				err,
			)
		}
		return 0, fmt.Errorf(
			"%w: DebitFees() call error: %w",
			ErrFeeCurrencyEVMCall,
			err,
		)
	}

	gasUsed := maxIntrinsicGasCost - leftoverGas
	log.Trace("DebitFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)
	return gasUsed, err
}

// Credits fees to the respective parties
// - the base fee goes to the fee handler
// - the transaction tip goes to the miner
// - the l1 data fee goes the the data fee receiver, is the node runs in rollup mode
// - remaining funds are refunded to the transaction sender
func CreditFees(
	evm *vm.EVM,
	feeCurrency *common.Address,
	txSender, tipReceiver, baseFeeReceiver, l1DataFeeReceiver common.Address,
	refund, feeTip, baseFee, l1DataFee *big.Int,
	gasUsedDebit uint64,
) error {
	// Hide this function from traces
	if evm.Config.Tracer != nil && !evm.Config.Tracer.TraceDebitCredit {
		origTracer := evm.Config.Tracer
		defer func() {
			evm.Config.Tracer = origTracer
		}()
		evm.Config.Tracer = nil
	}

	// Our old `creditGasFees` function does not accept an l1DataFee and
	// the fee currencies do not implement the new interface yet. Since tip
	// and data fee both go to the sequencer, we can work around that for
	// now by addint the l1DataFee to the tip.
	if l1DataFee != nil {
		feeTip = new(big.Int).Add(feeTip, l1DataFee)
	}

	// Not all fee currencies can handle a receiver being the zero address.
	// In that case send the fee to the base fee recipient, which we know is non-zero.
	if tipReceiver.Cmp(common.ZeroAddress) == 0 {
		tipReceiver = baseFeeReceiver
	}
	maxAllowedGasForDebitAndCredit, ok := common.MaxAllowedIntrinsicGasCost(evm.Context.FeeCurrencyContext.IntrinsicGasCosts, feeCurrency)
	if !ok {
		return fmt.Errorf("%w: %x", exchange.ErrUnregisteredFeeCurrency, feeCurrency)
	}

	maxAllowedGasForCredit := maxAllowedGasForDebitAndCredit - gasUsedDebit
	leftoverGas, err := evm.CallWithABI(
		feeCurrencyABI, "creditGasFees", *feeCurrency, maxAllowedGasForCredit,
		// function creditGasFees(
		// 	address from,
		// 	address feeRecipient,
		// 	address, // gatewayFeeRecipient, unused
		// 	address communityFund,
		// 	uint256 refund,
		// 	uint256 tipTxFee,
		// 	uint256, // gatewayFee, unused
		// 	uint256 baseTxFee
		// )
		txSender, tipReceiver, common.ZeroAddress, baseFeeReceiver, refund, feeTip, common.Big0, baseFee,
	)
	if err != nil {
		if errors.Is(err, vm.ErrOutOfGas) {
			// This is a configuration / contract error, since
			// the contract itself used way more gas than was expected (including grace limit)
			return fmt.Errorf(
				"%w: surpassed maximum allowed intrinsic gas for CreditFees() in fee-currency: %w",
				ErrFeeCurrencyEVMCall,
				err,
			)
		}
		return fmt.Errorf(
			"%w: CreditFees() call error: %w",
			ErrFeeCurrencyEVMCall,
			err,
		)
	}

	gasUsed := maxAllowedGasForCredit - leftoverGas
	log.Trace("CreditFees called", "feeCurrency", *feeCurrency, "gasUsed", gasUsed)

	intrinsicGas, ok := common.CurrencyIntrinsicGasCost(evm.Context.FeeCurrencyContext.IntrinsicGasCosts, feeCurrency)
	if !ok {
		// this will never happen
		return fmt.Errorf("%w: %x", exchange.ErrUnregisteredFeeCurrency, feeCurrency)
	}
	gasUsedForDebitAndCredit := gasUsedDebit + gasUsed
	if gasUsedForDebitAndCredit > intrinsicGas {
		log.Info(
			"Gas usage for debit+credit exceeds intrinsic gas!",
			"gasUsed", gasUsedForDebitAndCredit,
			"intrinsicGas", intrinsicGas,
			"feeCurrency", feeCurrency,
		)
	}
	return err
}

func GetRegisteredCurrencies(caller *abigen.FeeCurrencyDirectoryCaller) ([]common.Address, error) {
	currencies, err := caller.GetCurrencies(&bind.CallOpts{})
	if err != nil {
		return currencies, fmt.Errorf("failed to get registered tokens: %w", err)
	}
	return currencies, nil
}

// GetExchangeRates returns the exchange rates for the provided gas currencies
func GetExchangeRates(caller *CeloBackend) (common.ExchangeRates, error) {
	directory, err := abigen.NewFeeCurrencyDirectoryCaller(addresses.GetAddresses(caller.ChainConfig.ChainID).FeeCurrencyDirectory, caller)
	if err != nil {
		return common.ExchangeRates{}, fmt.Errorf("failed to access FeeCurrencyDirectory: %w", err)
	}
	currencies, err := GetRegisteredCurrencies(directory)
	if err != nil {
		return common.ExchangeRates{}, err
	}
	return getExchangeRatesForTokens(directory, currencies)
}

// GetFeeCurrencyContext returns the fee currency block context for all registered gas currencies from CELO
func GetFeeCurrencyContext(caller *CeloBackend) (common.FeeCurrencyContext, error) {
	var feeContext common.FeeCurrencyContext
	directory, err := abigen.NewFeeCurrencyDirectoryCaller(addresses.GetAddresses(caller.ChainConfig.ChainID).FeeCurrencyDirectory, caller)
	if err != nil {
		return feeContext, fmt.Errorf("failed to access FeeCurrencyDirectory: %w", err)
	}

	currencies, err := GetRegisteredCurrencies(directory)
	if err != nil {
		return feeContext, err
	}
	rates, err := getExchangeRatesForTokens(directory, currencies)
	if err != nil {
		return feeContext, err
	}
	intrinsicGas, err := getIntrinsicGasForTokens(directory, currencies)
	if err != nil {
		return feeContext, err
	}
	return common.FeeCurrencyContext{
		ExchangeRates:     rates,
		IntrinsicGasCosts: intrinsicGas,
	}, nil
}

// GetBalanceERC20 returns an account's balance on a given ERC20 currency
func GetBalanceERC20(caller bind.ContractCaller, accountOwner common.Address, contractAddress common.Address) (result *big.Int, err error) {
	token, err := abigen.NewFeeCurrencyCaller(contractAddress, caller)
	if err != nil {
		return nil, fmt.Errorf("failed to access FeeCurrency: %w", err)
	}

	balance, err := token.BalanceOf(&bind.CallOpts{}, accountOwner)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// GetFeeBalance returns the account's balance from the specified feeCurrency
// (if feeCurrency is nil or ZeroAddress, native currency balance is returned).
func GetFeeBalance(backend *CeloBackend, account common.Address, feeCurrency *common.Address) *big.Int {
	if feeCurrency == nil || *feeCurrency == common.ZeroAddress {
		return backend.State.GetBalance(account).ToBig()
	}
	balance, err := GetBalanceERC20(backend, account, *feeCurrency)
	if err != nil {
		log.Error("Error while trying to get ERC20 balance:", "cause", err, "contract", feeCurrency.Hex(), "account", account.Hex())
	}
	return balance
}

// getIntrinsicGasForTokens returns the intrinsic gas costs for the provided gas currencies from CELO
func getIntrinsicGasForTokens(caller *abigen.FeeCurrencyDirectoryCaller, tokens []common.Address) (common.IntrinsicGasCosts, error) {
	gasCosts := common.IntrinsicGasCosts{}
	for _, tokenAddress := range tokens {
		config, err := caller.GetCurrencyConfig(&bind.CallOpts{}, tokenAddress)
		if err != nil {
			log.Error("Failed to get intrinsic gas cost for gas currency!", "err", err, "tokenAddress", tokenAddress.Hex())
			continue
		}
		if !config.IntrinsicGas.IsUint64() {
			log.Error("Intrinsic gas cost exceeds MaxUint64 limit, capping at MaxUint64", "err", err, "tokenAddress", tokenAddress.Hex())
			gasCosts[tokenAddress] = math.MaxUint64
		} else {
			gasCosts[tokenAddress] = config.IntrinsicGas.Uint64()
		}
	}
	return gasCosts, nil
}

// getExchangeRatesForTokens returns the exchange rates for the provided gas currencies from CELO
func getExchangeRatesForTokens(caller *abigen.FeeCurrencyDirectoryCaller, tokens []common.Address) (common.ExchangeRates, error) {
	exchangeRates := common.ExchangeRates{}
	for _, tokenAddress := range tokens {
		rate, err := caller.GetExchangeRate(&bind.CallOpts{}, tokenAddress)
		if err != nil {
			log.Error("Failed to get medianRate for gas currency!", "err", err, "tokenAddress", tokenAddress.Hex())
			continue
		}
		if rate.Numerator.Sign() <= 0 || rate.Denominator.Sign() <= 0 {
			log.Error("Bad exchange rate for fee currency", "tokenAddress", tokenAddress.Hex(), "numerator", rate.Numerator, "denominator", rate.Denominator)
			continue
		}
		exchangeRates[tokenAddress] = new(big.Rat).SetFrac(rate.Numerator, rate.Denominator)
	}

	return exchangeRates, nil
}
