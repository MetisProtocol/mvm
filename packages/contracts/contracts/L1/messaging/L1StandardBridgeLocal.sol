// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Interface Imports */
import { IL1StandardBridge } from "./IL1StandardBridge.sol";
import { IL1ERC20Bridge } from "./IL1ERC20Bridge.sol";
import { IL2ERC20Bridge } from "../../L2/messaging/IL2ERC20Bridge.sol";
import { IERC20 } from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

/* Library Imports */
import { CrossDomainEnabled } from "../../libraries/bridge/CrossDomainEnabled.sol";
import { Lib_PredeployAddresses } from "../../libraries/constants/Lib_PredeployAddresses.sol";
import { Address } from "@openzeppelin/contracts/utils/Address.sol";
import { SafeERC20 } from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import { iMVM_DiscountOracle } from "../../MVM/iMVM_DiscountOracle.sol";
import { Lib_AddressManager } from "../../libraries/resolver/Lib_AddressManager.sol";

/**
 * @title L1StandardBridgeLocal
 * @dev The L1 ETH and ERC20 Bridge is a contract which stores deposited L1 funds and standard
 * tokens that are in use on L2. It synchronizes a corresponding L2 Bridge, informing it of deposits
 * and listening to it for newly finalized withdrawals.
 *
 * Runtime target: EVM
 */
contract L1StandardBridgeLocal is IL1StandardBridge, CrossDomainEnabled {
    using SafeERC20 for IERC20;

    /********************************
     * External Contract References *
     ********************************/

    address public l2TokenBridge;
    address public metis;
    address public addressmgr;

    // Maps L1 token to chainid to L2 token to balance of the L1 token deposited
    mapping(address => mapping (uint256 => mapping (address => uint256))) public deposits;

    
    uint256 constant public DEFAULT_CHAINID = 1088;

    /***************
     * Constructor *
     ***************/

    // This contract lives behind a proxy, so the constructor parameters will go unused.
    constructor() CrossDomainEnabled(address(0)) {}

    /******************
     * Initialization *
     ******************/

    /**
     * @param _l1messenger L1 Messenger address being used for cross-chain communications.
     * @param _l2TokenBridge L2 standard bridge address.
     */
    function initialize(
        address _l1messenger,
        address _l2TokenBridge,
        address _metis,
        address _addressmgr
    )
        public
    {
        require(messenger == address(0), "Contract has already been initialized.");
        messenger = _l1messenger;
        l2TokenBridge = _l2TokenBridge;
        metis = _metis;
        addressmgr = _addressmgr;
    }

    /**************
     * Depositing *
     **************/

    /** @dev Modifier requiring sender to be EOA.  This check could be bypassed by a malicious
     *  contract via initcode, but it takes care of the user error we want to avoid.
     */
    modifier onlyEOA() {
        // Used to stop deposits from contracts (avoid accidentally lost tokens)
        require(!Address.isContract(msg.sender), "Account not EOA");
        _;
    }

    /**
     * @dev do not accept no data call
     */
    receive() external payable onlyEOA {
        require(false, "do not send eth directly");
    }

    /**
     * @inheritdoc IL1StandardBridge
     */
    function depositETH(uint32 _l2Gas, bytes calldata _data) external payable onlyEOA {
        _initiateETHDeposit(msg.sender, msg.sender, _l2Gas, _data);
    }
    
    function depositETHByChainId(
        uint256 _chainId,
        uint32 _l2Gas,
        bytes calldata _data
    )
        external
        override
        payable
        onlyEOA()
    {
        _initiateETHDepositByChainId(
            _chainId,
            msg.sender,
            msg.sender,
            _l2Gas,
            _data
        );
    }

    /**
     * @inheritdoc IL1StandardBridge
     */
    function depositETHTo(
        address _to,
        uint32 _l2Gas,
        bytes calldata _data
    ) external payable {
        _initiateETHDeposit(msg.sender, _to, _l2Gas, _data);
    }
    
    function depositETHToByChainId(
        uint256 _chainId,
        address _to,
        uint32 _l2Gas,
        bytes calldata _data
    )
        external
        override
        payable
    {
        _initiateETHDepositByChainId(
            _chainId,
            msg.sender,
            _to,
            _l2Gas,
            _data
        );
    }

    /**
     * @dev Performs the logic for deposits by storing the ETH and informing the L2 ETH Gateway of
     * the deposit.
     * @param _from Account to pull the deposit from on L1.
     * @param _to Account to give the deposit to on L2.
     * @param _l2Gas Gas limit required to complete the deposit on L2.
     * @param _data Optional data to forward to L2. This data is provided
     *        solely as a convenience for external contracts. Aside from enforcing a maximum
     *        length, these contracts provide no guarantees about its content.
     */
    function _initiateETHDeposit(
        address _from,
        address _to,
        uint32 _l2Gas,
        bytes memory _data
    ) internal {
        _initiateETHDepositByChainId(DEFAULT_CHAINID, _from, _to, _l2Gas, _data);
    }
    
    function _initiateETHDepositByChainId(
        uint256 _chainId,
        address _from,
        address _to,
        uint32 _l2Gas,
        bytes memory _data
    )
        internal
    {
    
        iMVM_DiscountOracle oracle = iMVM_DiscountOracle(Lib_AddressManager(addressmgr).getAddress('MVM_DiscountOracle'));
        uint32 mingas = uint32(oracle.getMinL2Gas());
        if (_l2Gas < mingas) {
            _l2Gas = mingas;
        }
        uint256 fee = _l2Gas / oracle.getDiscount();
        
        require(fee <= msg.value, string(abi.encodePacked("insufficient fee supplied. send at least ", uint2str(fee))));
        // Construct calldata for finalizeDeposit call
        bytes memory message =
            abi.encodeWithSelector(
                IL2ERC20Bridge.finalizeDeposit.selector,
                address(0),
                Lib_PredeployAddresses.MVM_COINBASE,  //MVM: only for local dev environment for easier funding
                _from,
                _to,
                msg.value - fee,
                _data
            );
        
        // Send calldata into L2
        sendCrossDomainMessageViaChainId(
            _chainId,
            l2TokenBridge,
            _l2Gas,
            message,
            fee  // only send the supplied fees over (obviously)
        );

        emit ETHDepositInitiated(_from, _to, msg.value, _data, _chainId);
    }

    /**
     * @inheritdoc IL1ERC20Bridge
     */
    function depositERC20(
        address _l1Token,
        address _l2Token,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    ) external virtual onlyEOA {
        _initiateERC20DepositByChainId(DEFAULT_CHAINID, _l1Token, _l2Token, msg.sender, msg.sender, _amount, _l2Gas, _data);
    }

    /**
     * @inheritdoc IL1ERC20Bridge
     */
    function depositERC20To(
        address _l1Token,
        address _l2Token,
        address _to,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    ) external virtual {
        _initiateERC20DepositByChainId(DEFAULT_CHAINID, _l1Token, _l2Token, msg.sender, _to, _amount, _l2Gas, _data);
    }
    function depositERC20ByChainId(
        uint256 _chainid,
        address _l1Token,
        address _l2Token,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    )
        external
        override
        virtual
        payable
        onlyEOA()
    {
        _initiateERC20DepositByChainId(_chainid, _l1Token, _l2Token, msg.sender, msg.sender, _amount, _l2Gas, _data);
    }
    
    function depositERC20ToByChainId(
        uint256 _chainid,
        address _l1Token,
        address _l2Token,
        address _to,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    )
        external
        override
        payable
        virtual
    {
        _initiateERC20DepositByChainId(_chainid, _l1Token, _l2Token, msg.sender, _to, _amount, _l2Gas, _data);
    }

    /**
     * @dev Performs the logic for deposits by informing the L2 Deposited Token
     * contract of the deposit and calling a handler to lock the L1 funds. (e.g. transferFrom)
     *
     * @param _l1Token Address of the L1 ERC20 we are depositing
     * @param _l2Token Address of the L1 respective L2 ERC20
     * @param _from Account to pull the deposit from on L1
     * @param _to Account to give the deposit to on L2
     * @param _amount Amount of the ERC20 to deposit.
     * @param _l2Gas Gas limit required to complete the deposit on L2.
     * @param _data Optional data to forward to L2. This data is provided
     *        solely as a convenience for external contracts. Aside from enforcing a maximum
     *        length, these contracts provide no guarantees about its content.
     */
    function _initiateERC20Deposit(
        address _l1Token,
        address _l2Token,
        address _from,
        address _to,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    ) internal {
        _initiateERC20DepositByChainId(DEFAULT_CHAINID, _l1Token, _l2Token, _from, _to, _amount, _l2Gas, _data);
    }
    
    function _initiateERC20DepositByChainId(
        uint256 _chainId,
        address _l1Token,
        address _l2Token,
        address _from,
        address _to,
        uint256 _amount,
        uint32 _l2Gas,
        bytes calldata _data
    )
        internal
    {
        iMVM_DiscountOracle oracle = iMVM_DiscountOracle(Lib_AddressManager(addressmgr).getAddress('MVM_DiscountOracle'));
        
        // stack too deep. so no more local variables
        if (_l2Gas < uint32(oracle.getMinL2Gas())) {
            _l2Gas = uint32(oracle.getMinL2Gas());
        }
        
        require(_l2Gas * oracle.getDiscount() <= msg.value, 
                string(abi.encodePacked("insufficient fee supplied. send at least ", uint2str(_l2Gas * oracle.getDiscount()))));
        
        // When a deposit is initiated on L1, the L1 Bridge transfers the funds to itself for future
        // withdrawals. safeTransferFrom also checks if the contract has code, so this will fail if
        // _from is an EOA or address(0).
        IERC20(_l1Token).safeTransferFrom(
            _from,
            address(this),
            _amount
        );
        
        bytes memory message;
        if (_l1Token == metis) {
            // Construct calldata for finalizeDeposit call
          _l2Token = Lib_PredeployAddresses.MVM_COINBASE;
          message =
            abi.encodeWithSelector(
                IL2ERC20Bridge.finalizeDeposit.selector,
                address(0),
                Lib_PredeployAddresses.MVM_COINBASE,
                _from,
                _to,
                _amount,
                _data
            );
            
        } else {
          // Construct calldata for finalizeDeposit call
          message =
            abi.encodeWithSelector(
                IL2ERC20Bridge.finalizeDeposit.selector,
                _l1Token,
                _l2Token,
                _from,
                _to,
                _amount,
                _data
            );
        }

        // Send calldata into L2
        sendCrossDomainMessageViaChainId(
            _chainId,
            l2TokenBridge,
            _l2Gas,
            message,
            msg.value  //send all values as fees to cover l2 tx cost
        );
        
        deposits[_l1Token][_chainId][_l2Token] = deposits[_l1Token][_chainId][_l2Token] + (_amount);

        emit ERC20ChainID(_chainId);
        emit ERC20DepositInitiated(_l1Token, _l2Token, _from, _to, _amount, _data);
        
    }

    /*************************
     * Cross-chain Functions *
     *************************/

    /**
     * @inheritdoc IL1StandardBridge
     */
    function finalizeETHWithdrawal(
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    ) external onlyFromCrossDomainAccount(l2TokenBridge) {
        (bool success, ) = _to.call{ value: _amount }(new bytes(0));
        require(success, "TransferHelper::safeTransferETH: ETH transfer failed");

        emit ETHWithdrawalFinalized(_from, _to, _amount, _data, DEFAULT_CHAINID);
    }
    
    function finalizeETHWithdrawalByChainId(
        uint256 _chainid,
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    )
        external
        override
        onlyFromCrossDomainAccount(l2TokenBridge)
    {
        (bool success, ) = _to.call{value: _amount}(new bytes(0));
        require(success, "TransferHelper::safeTransferETH: ETH transfer failed");

        emit ETHWithdrawalFinalized(_from, _to, _amount, _data, _chainid);
    }
    
    function finalizeMetisWithdrawalByChainId(
        uint256 _chainid,
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    )
        external
        override
        onlyFromCrossDomainAccount(l2TokenBridge)
    {
        _finalizeERC20WithdrawalByChainId(_chainid, metis, Lib_PredeployAddresses.MVM_COINBASE, _from, _to, _amount, _data);
    }

    /**
     * @inheritdoc IL1ERC20Bridge
     */
    function finalizeERC20Withdrawal(
        address _l1Token,
        address _l2Token,
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    ) external onlyFromCrossDomainAccount(l2TokenBridge) {
        _finalizeERC20WithdrawalByChainId(DEFAULT_CHAINID, _l1Token, _l2Token, _from, _to, _amount, _data);
    }
    
    function finalizeERC20WithdrawalByChainId(
        uint256 _chainid,
        address _l1Token,
        address _l2Token,
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    )
        external
        override
        onlyFromCrossDomainAccount(l2TokenBridge)
    {
        _finalizeERC20WithdrawalByChainId(_chainid, _l1Token, _l2Token, _from, _to, _amount, _data);
    }
    
    function _finalizeERC20WithdrawalByChainId(
        uint256 _chainid,
        address _l1Token,
        address _l2Token,
        address _from,
        address _to,
        uint256 _amount,
        bytes calldata _data
    )
        internal
    {
        deposits[_l1Token][_chainid][_l2Token] = deposits[_l1Token][_chainid][_l2Token] - _amount;

        // When a withdrawal is finalized on L1, the L1 Bridge transfers the funds to the withdrawer
        IERC20(_l1Token).safeTransfer(_to, _amount);

        emit ERC20ChainID(_chainid);
        emit ERC20WithdrawalFinalized(_l1Token, _l2Token, _from, _to, _amount, _data);
        
    }

    /*****************************
     * Temporary - Migrating ETH *
     *****************************/

    /**
     * @dev Adds ETH balance to the account. This is meant to allow for ETH
     * to be migrated from an old gateway to a new gateway.
     * NOTE: This is left for one upgrade only so we are able to receive the migrated ETH from the
     * old contract
     */
    function donateETH() external payable {}
    
    function uint2str(uint _i) internal pure returns (string memory _uintAsString) {
        if (_i == 0) {
            return "0";
        }
        uint j = _i;
        uint len;
        while (j != 0) {
            len++;
            j /= 10;
        }
        bytes memory bstr = new bytes(len);
        uint k = len;
        while (_i != 0) {
            k = k-1;
            uint8 temp = (48 + uint8(_i - _i / 10 * 10));
            bytes1 b1 = bytes1(temp);
            bstr[k] = b1;
            _i /= 10;
        }
        return string(bstr);
    }
}
