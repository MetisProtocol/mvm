// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { MVM_Coinbase } from "./MVM_Coinbase.sol";
import { Lib_PredeployAddresses } from "../libraries/constants/Lib_PredeployAddresses.sol";
contract MVM_GasOracle is Ownable{
    // Current l2 gas price
    uint256 public gasPrice;
    uint256 public minL1GasLimit;

    constructor(
      address _owner,
      uint256 _initialGasPrice
    )
      Ownable() 
    {
      setGasPrice(_initialGasPrice);
      transferOwnership(_owner);
    }

    function setGasPrice(
        uint256 _gasPrice
    )
        public
        onlyOwner
    {
        gasPrice = _gasPrice;
    }
    
    
    function setMinL1GasLimit(
        uint256 gas
    )
        public
        onlyOwner
    {
        minL1GasLimit = gas;
    }

    //for compatibility
    function setPrice(uint256 _gasPrice) public onlyOwner{
       gasPrice = _gasPrice;
    }

    function transferSetter(address newsetter) public onlyOwner{
       transferOwnership(newsetter);
    }

    function transferTo(address target, uint256 amount) public onlyOwner{
        // Transfer fee to relayer.
        require(
            MVM_Coinbase(Lib_PredeployAddresses.MVM_COINBASE).transfer(
                target,
                amount
            ),
            "transfer failed."
        );
    }
}