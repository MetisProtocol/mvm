// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

/// @title ILockingPool
/// @notice Interface for the LockingPool contract
interface ILockingPool {
    /// @notice Slashes a percentage of tokens from the pool
    /// @param _recipient Address to receive the slashed tokens
    function slash(address _recipient) external;
} 