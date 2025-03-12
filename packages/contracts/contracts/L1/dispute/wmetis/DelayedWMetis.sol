// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import {MetisConfig} from "../../config/MetisConfig.sol";
import {ISemver} from "../../../universal/ISemver.sol";
import {IDelayedWMetis} from "../interfaces/IDelayedWMetis.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {Initializable} from "@openzeppelin/contracts/proxy/utils/Initializable.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

/// @title DelayedWMetis
/// @notice DelayedWMetis is a wrapper for Metis token that allows for delayed withdrawals.
/// @dev This contract follows the same pattern as DelayedWETH but for Metis ERC20 token
contract DelayedWMetis is OwnableUpgradeable, IDelayedWMetis, ISemver {
    /// @notice Semantic version.
    /// @custom:semver 1.0.0
    string public constant version = "1.0.0";

    /// @notice The Metis token contract
    IERC20 internal immutable METIS;

    /// @notice Withdrawal delay in seconds.
    uint256 internal immutable DELAY_SECONDS;

    /// @notice Address of the MetisConfig contract.
    MetisConfig public config;

    /// @notice Mapping of account balances
    mapping(address => uint256) public balanceOf;

    /// @notice Mapping of account allowances
    mapping(address => mapping(address => uint256)) public allowance;

    /// @inheritdoc IDelayedWMetis
    mapping(address => mapping(address => WithdrawalRequest)) public withdrawals;

    uint8 public constant decimals = 18;

    /// @param _delay The delay for withdrawals in seconds.
    /// @param _metis The Metis token contract address
    constructor(uint256 _delay, IERC20 _metis) {
        DELAY_SECONDS = _delay;
        METIS = _metis;
        initialize({_owner: address(0), _config: MetisConfig(address(0))});
    }

    /// @notice Initializes the contract.
    /// @param _owner The address of the owner.
    /// @param _config Address of the MetisConfig contract.
    function initialize(address _owner, MetisConfig _config) public initializer {
        __Ownable_init();
        _transferOwnership(_owner);
        config = _config;
    }

    /// @inheritdoc IDelayedWMetis
    function delay() external view returns (uint256) {
        return DELAY_SECONDS;
    }

    function name() external pure returns (string memory) {
        return "Wrapped Metis";
    }

    function symbol() external pure returns (string memory) {
        return "WMETIS";
    }

    function totalSupply() external view returns (uint256) {
        return METIS.balanceOf(address(this));
    }

    /// @inheritdoc IDelayedWMetis
    function deposit(uint256 _amount) external {
        METIS.transferFrom(msg.sender, address(this), _amount);
        balanceOf[msg.sender] += _amount;
        emit Deposit(msg.sender, _amount);
    }

    /// @inheritdoc IDelayedWMetis
    function unlock(address _guy, uint256 _amount) external {
        WithdrawalRequest storage wd = withdrawals[msg.sender][_guy];
        wd.timestamp = block.timestamp;
        wd.amount += _amount;
        emit Unlock(msg.sender, _guy, _amount);
    }

    /// @inheritdoc IDelayedWMetis
    function withdraw(uint256 _amount) external {
        withdraw(msg.sender, _amount);
    }

    /// @inheritdoc IDelayedWMetis
    function withdraw(address _guy, uint256 _amount) public {
        require(!config.paused(), "DelayedWMetis: contract is paused");
        WithdrawalRequest storage wd = withdrawals[msg.sender][_guy];
        require(wd.amount >= _amount, "DelayedWMetis: insufficient unlocked withdrawal");
        require(wd.timestamp > 0, "DelayedWMetis: withdrawal not unlocked");
        require(wd.timestamp + DELAY_SECONDS <= block.timestamp, "DelayedWMetis: withdrawal delay not met");
        require(balanceOf[msg.sender] >= _amount, "DelayedWMetis: insufficient balance");

        wd.amount -= _amount;
        balanceOf[msg.sender] -= _amount;
        METIS.transfer(msg.sender, _amount);
        emit Withdrawal(msg.sender, _amount);
    }

    /// @inheritdoc IDelayedWMetis
    function recover(uint256 _amount) external {
        require(msg.sender == owner(), "DelayedWMetis: not owner");
        uint256 balance = METIS.balanceOf(address(this));
        uint256 amount = _amount < balance ? _amount : balance;
        METIS.transfer(msg.sender, amount);
    }

    /// @inheritdoc IDelayedWMetis
    function hold(address _guy, uint256 _amount) external {
        require(msg.sender == owner(), "DelayedWMetis: not owner");
        allowance[_guy][msg.sender] = _amount;
        emit Approval(_guy, msg.sender, _amount);
    }

    function approve(address guy, uint256 wad) external returns (bool) {
        allowance[msg.sender][guy] = wad;
        emit Approval(msg.sender, guy, wad);
        return true;
    }

    function transfer(address dst, uint256 wad) external returns (bool) {
        return transferFrom(msg.sender, dst, wad);
    }

    function transferFrom(address src, address dst, uint256 wad) public returns (bool) {
        require(balanceOf[src] >= wad, "DelayedWMetis: insufficient balance");

        if (src != msg.sender && allowance[src][msg.sender] != type(uint256).max) {
            require(allowance[src][msg.sender] >= wad, "DelayedWMetis: insufficient allowance");
            allowance[src][msg.sender] -= wad;
        }

        balanceOf[src] -= wad;
        balanceOf[dst] += wad;

        emit Transfer(src, dst, wad);

        return true;
    }

    /// @inheritdoc IDelayedWMetis
    function metisToken() external view override returns (IERC20) {
        return METIS;
    }
} 