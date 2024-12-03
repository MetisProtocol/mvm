// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import {IFaultDisputeLockingPool} from "./IFaultDisputeLockingPool.sol";
import {OwnableUpgradeable} from "@openzeppelin/contracts-upgradeable/access/OwnableUpgradeable.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

/// @title FaultDisputeLockingPool
/// @notice A contract that allows users to lock and unlock ERC20 tokens with delayed withdrawals.
contract FaultDisputeLockingPool is OwnableUpgradeable, IFaultDisputeLockingPool {
    /// @notice The ERC20 token that this vault accepts.
    IERC20 public token;

    /// @notice The delay in seconds before a withdrawal can be processed after unlocking.
    uint256 public immutable DELAY_SECONDS;

    /// @notice Tracks the token balance of each user within the vault.
    mapping(address => uint256) public balanceOf;

    /// @notice Stores withdrawal requests for delayed withdrawals.
    struct WithdrawalRequest {
        uint256 amount;
        uint256 timestamp;
    }

    /// @notice Maps users to their withdrawal requests for each beneficiary.
    mapping(address => mapping(address => WithdrawalRequest)) public withdrawals;

    /// @notice Event emitted when tokens are deposited into the vault.
    event Deposit(address indexed account, uint256 amount);

    /// @notice Event emitted when tokens are withdrawn from the vault.
    event Withdrawal(address indexed account, uint256 amount);

    /// @param _token The ERC20 token address that this vault will accept.
    /// @param _delay The delay in seconds for withdrawals after unlocking.
    constructor(IERC20 _token, uint256 _delay) {
        token = _token;
        DELAY_SECONDS = _delay;
        initialize(msg.sender);
    }

    /// @notice Initializes the contract and sets the owner.
    /// @param _owner The address of the contract owner.
    function initialize(address _owner) public initializer {
        __Ownable_init();
        _transferOwnership(_owner);
    }

    /// @inheritdoc IFaultDisputeLockingPool
    function deposit(uint256 amount) external override {
        require(
            token.transferFrom(msg.sender, address(this), amount),
            "FaultDisputeLockingPool: token transfer failed"
        );
        balanceOf[msg.sender] += amount;
        emit Deposit(msg.sender, amount);
    }

    /// @inheritdoc IFaultDisputeLockingPool
    function unlock(address beneficiary, uint256 amount) external override {
        require(
            balanceOf[msg.sender] >= amount,
            "FaultDisputeLockingPool: insufficient balance to unlock"
        );
        WithdrawalRequest storage wd = withdrawals[msg.sender][beneficiary];
        wd.timestamp = block.timestamp;
        wd.amount += amount;
    }

    /// @inheritdoc IFaultDisputeLockingPool
    function withdraw(uint256 amount) external override {
        withdraw(msg.sender, amount);
    }

    /// @inheritdoc IFaultDisputeLockingPool
    function withdraw(address beneficiary, uint256 amount) public override {
        WithdrawalRequest storage wd = withdrawals[msg.sender][beneficiary];
        require(
            wd.amount >= amount,
            "DelayedTokenVault: insufficient unlocked amount"
        );
        require(
            wd.timestamp + DELAY_SECONDS <= block.timestamp,
            "FaultDisputeLockingPool: withdrawal delay not met"
        );

        wd.amount -= amount;
        balanceOf[msg.sender] -= amount;

        require(
            token.transfer(beneficiary, amount),
            "FaultDisputeLockingPool: token transfer failed"
        );

        emit Withdrawal(beneficiary, amount);
    }
}
