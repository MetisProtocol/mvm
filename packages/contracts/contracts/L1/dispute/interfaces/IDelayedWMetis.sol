// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";

/// @title IDelayedWMetis
/// @notice Interface for the DelayedWMetis contract
interface IDelayedWMetis is IERC20 {
    /// @notice Struct representing a withdrawal request
    struct WithdrawalRequest {
        uint256 amount;
        uint256 timestamp;
    }

    /// @notice Emitted when tokens are deposited
    event Deposit(address indexed user, uint256 amount);

    /// @notice Emitted when a withdrawal is unlocked
    event Unlock(address indexed user, address indexed recipient, uint256 amount);

    /// @notice Emitted when tokens are withdrawn
    event Withdrawal(address indexed user, uint256 amount);

    /// @notice Returns the delay period for withdrawals
    function delay() external view returns (uint256);

    /// @notice Returns the withdrawal request for a given user and recipient
    /// @param _user The user address
    /// @param _recipient The recipient address
    function withdrawals(address _user, address _recipient) external view returns (uint256, uint256);

    /// @notice Unlocks tokens for withdrawal
    /// @param _guy The recipient address
    /// @param _amount The amount to unlock
    function unlock(address _guy, uint256 _amount) external;

    /// @notice Deposits Metis tokens into the contract
    /// @param _amount Amount of Metis tokens to deposit
    function deposit(uint256 _amount) external;

    /// @notice Extension to withdrawal, must provide a sub-account to withdraw from.
    /// @param _guy Sub-account to withdraw from.
    /// @param _amount The amount of WETH to withdraw.
    function withdraw(address _guy, uint256 _amount) external;

    /// @notice Withdraws tokens to msg.sender
    /// @param _amount The amount to withdraw
    function withdraw(uint256 _amount) external;

    /// @notice Allows the owner to hold tokens from a user
    /// @param _guy The user address
    /// @param _amount The amount to hold
    function hold(address _guy, uint256 _amount) external;

    /// @notice Allows the owner to recover tokens from the contract
    /// @param _amount The amount to recover
    function recover(uint256 _amount) external;

    /// @notice Returns the underlying Metis token contract address
    function metisToken() external view returns (IERC20);
} 